package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rushikeshg25/runnelix/internal/definition"
)

type Workflow struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Revision        int64           `json:"revision"`
	Graph           json.RawMessage `json:"graph"`
	ActiveVersionID *string         `json:"activeVersionId"`
}
type Version struct {
	ID            string          `json:"id"`
	WorkflowID    string          `json:"workflowId"`
	Sequence      int64           `json:"sequence"`
	DraftRevision int64           `json:"draftRevision"`
	Graph         json.RawMessage `json:"graph"`
	Plan          json.RawMessage `json:"plan"`
	Digest        string          `json:"digest"`
}

func (s *Store) CreateWorkspace(ctx context.Context, id, name string) error {
	if !validID(id) || len(strings.TrimSpace(name)) == 0 || len(name) > 120 {
		return ErrInvalid
	}
	_, err := s.Pool.Exec(ctx, "INSERT INTO workspaces(id,name) VALUES($1,$2) ON CONFLICT(id) DO NOTHING", id, name)
	return err
}
func (s *Store) CreateWorkflow(ctx context.Context, workspace, name string, graph []byte) (Workflow, error) {
	if len(strings.TrimSpace(name)) == 0 || len(name) > 120 {
		return Workflow{}, ErrInvalid
	}
	// Drafts may have incomplete edges. The format remains strict; publication validates semantics.
	if err := validateDraft(graph); err != nil {
		return Workflow{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Workflow{}, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM workspaces WHERE id=$1)", workspace).Scan(&exists); err != nil {
		return Workflow{}, err
	}
	if !exists {
		return Workflow{}, ErrNotFound
	}
	id := "workflow_" + uuid.NewString()
	if _, err = tx.Exec(ctx, "INSERT INTO workflows(workspace_id,id,name) VALUES($1,$2,$3)", workspace, id, name); err != nil {
		return Workflow{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO workflow_drafts(workspace_id,workflow_id,graph) VALUES($1,$2,$3)", workspace, id, graph); err != nil {
		return Workflow{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Workflow{}, err
	}
	return s.GetWorkflow(ctx, workspace, id)
}
func validateDraft(graph []byte) error {
	var raw any
	if err := definition.Decode(graph, &raw); err != nil {
		return &definition.Invalid{Issues: []definition.Issue{{Path: "/graph", Code: "invalid_json", Message: err.Error()}}}
	}
	if err := definition.Schema("workflow.schema.json", raw); err != nil {
		return definition.SchemaError(err, "/graph")
	}
	return nil
}
func (s *Store) GetWorkflow(ctx context.Context, workspace, id string) (Workflow, error) {
	var w Workflow
	err := s.Pool.QueryRow(ctx, `SELECT w.id,w.name,d.revision,d.graph,w.active_version_id FROM workflows w JOIN workflow_drafts d ON (d.workspace_id,d.workflow_id)=(w.workspace_id,w.id) WHERE w.workspace_id=$1 AND w.id=$2`, workspace, id).Scan(&w.ID, &w.Name, &w.Revision, &w.Graph, &w.ActiveVersionID)
	return w, notFound(err)
}
func (s *Store) SaveDraft(ctx context.Context, workspace, id string, revision int64, graph []byte) (Workflow, error) {
	if revision < 1 {
		return Workflow{}, ErrInvalid
	}
	if err := validateDraft(graph); err != nil {
		return Workflow{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Workflow{}, err
	}
	defer tx.Rollback(ctx)
	// All workflow mutations lock the same parent first, avoiding inconsistent lock order.
	var found string
	if err = tx.QueryRow(ctx, "SELECT id FROM workflows WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspace, id).Scan(&found); err != nil {
		return Workflow{}, notFound(err)
	}
	tag, err := tx.Exec(ctx, "UPDATE workflow_drafts SET graph=$4,revision=revision+1 WHERE workspace_id=$1 AND workflow_id=$2 AND revision=$3 AND revision<9007199254740991", workspace, id, revision, graph)
	if err != nil {
		return Workflow{}, err
	}
	if tag.RowsAffected() != 1 {
		return Workflow{}, ErrConflict
	}
	// Read before releasing the lock so another writer cannot change the returned revision.
	var w Workflow
	err = tx.QueryRow(ctx, `SELECT w.id,w.name,d.revision,d.graph,w.active_version_id FROM workflows w JOIN workflow_drafts d ON (d.workspace_id,d.workflow_id)=(w.workspace_id,w.id) WHERE w.workspace_id=$1 AND w.id=$2`, workspace, id).Scan(&w.ID, &w.Name, &w.Revision, &w.Graph, &w.ActiveVersionID)
	if err != nil {
		return Workflow{}, err
	}
	return w, tx.Commit(ctx)
}
func (s *Store) Publish(ctx context.Context, workspace, id string, revision int64) (Version, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Version{}, err
	}
	defer tx.Rollback(ctx)
	var found string
	if err = tx.QueryRow(ctx, "SELECT id FROM workflows WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspace, id).Scan(&found); err != nil {
		return Version{}, notFound(err)
	}
	var current int64
	var graph []byte
	if err = tx.QueryRow(ctx, "SELECT revision,graph FROM workflow_drafts WHERE workspace_id=$1 AND workflow_id=$2", workspace, id).Scan(&current, &graph); err != nil {
		return Version{}, err
	}
	if revision != current {
		return Version{}, ErrConflict
	}
	var existingID string
	err = tx.QueryRow(ctx, "SELECT id FROM workflow_versions WHERE workspace_id=$1 AND workflow_id=$2 AND draft_revision=$3", workspace, id, revision).Scan(&existingID)
	if err == nil {
		v, e := getVersion(ctx, tx, workspace, id, existingID)
		if e != nil {
			return Version{}, e
		}
		return v, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Version{}, err
	}
	_, plan, digest, err := definition.Compile(graph, s.Catalog)
	if err != nil {
		return Version{}, err
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return Version{}, err
	}
	var sequence int64
	if err = tx.QueryRow(ctx, "SELECT coalesce(max(sequence),0)+1 FROM workflow_versions WHERE workspace_id=$1 AND workflow_id=$2", workspace, id).Scan(&sequence); err != nil {
		return Version{}, err
	}
	v := Version{ID: "version_" + uuid.NewString(), WorkflowID: id, Sequence: sequence, DraftRevision: revision, Graph: graph, Plan: planJSON, Digest: digest}
	if _, err = tx.Exec(ctx, "INSERT INTO workflow_versions(workspace_id,workflow_id,id,sequence,draft_revision,graph,plan,digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", workspace, id, v.ID, sequence, revision, graph, planJSON, digest); err != nil {
		return Version{}, err
	}
	if _, err = tx.Exec(ctx, "UPDATE workflows SET active_version_id=$3 WHERE workspace_id=$1 AND id=$2", workspace, id, v.ID); err != nil {
		return Version{}, err
	}
	return v, tx.Commit(ctx)
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getVersion(ctx context.Context, q querier, workspace, workflow, id string) (Version, error) {
	var v Version
	err := q.QueryRow(ctx, "SELECT id,workflow_id,sequence,draft_revision,graph,plan,digest FROM workflow_versions WHERE workspace_id=$1 AND workflow_id=$2 AND id=$3", workspace, workflow, id).Scan(&v.ID, &v.WorkflowID, &v.Sequence, &v.DraftRevision, &v.Graph, &v.Plan, &v.Digest)
	return v, notFound(err)
}
func (s *Store) GetVersion(ctx context.Context, workspace, workflow, id string) (Version, error) {
	return getVersion(ctx, s.Pool, workspace, workflow, id)
}
func validID(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for i, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && ((r >= '0' && r <= '9') || r == '_' || r == '-')) {
			continue
		}
		return false
	}
	return true
}
