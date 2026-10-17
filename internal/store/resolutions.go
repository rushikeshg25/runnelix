package store

import (
	"context"
	"encoding/json"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"strings"
)

type Resolution struct {
	WorkspaceID  string `json:"workspaceId"`
	RunID        string `json:"runId"`
	InvocationID string `json:"invocationId"`
	Action       string `json:"action"`
	OutputRef    string `json:"outputRef"`
	Actor        string `json:"actor"`
	Note         string `json:"note"`
}

func (s *Store) MarkUncertain(ctx context.Context, e definition.Envelope) error {
	if err := s.Invocation(ctx, e); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, "INSERT INTO uncertain_invocations(workspace_id,run_id,invocation_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", e.WorkspaceID, e.RunID, e.InvocationID)
	return err
}
func (s *Store) ResolveUncertain(ctx context.Context, res Resolution, output any) (Resolution, error) {
	if (res.Action != "confirm" && res.Action != "fail") || len(strings.TrimSpace(res.Note)) < 1 || len(res.Note) > 500 || res.Actor == "" || len(res.Actor) > 200 {
		return res, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, "SELECT state FROM runs WHERE workspace_id=$1 AND id=$2 FOR NO KEY UPDATE", res.WorkspaceID, res.RunID).Scan(&state); err != nil {
		return res, notFound(err)
	}
	if state == "succeeded" || state == "failed" || state == "cancelled" {
		return res, ErrConflict
	}
	var raw []byte
	var unresolved bool
	err = tx.QueryRow(ctx, `SELECT i.envelope,u.resolved_at IS NULL FROM uncertain_invocations u JOIN node_invocations i ON (i.workspace_id,i.run_id,i.id)=(u.workspace_id,u.run_id,u.invocation_id) WHERE u.workspace_id=$1 AND u.run_id=$2 AND u.invocation_id=$3`, res.WorkspaceID, res.RunID, res.InvocationID).Scan(&raw, &unresolved)
	if err != nil {
		return res, notFound(err)
	}
	if !unresolved {
		return res, ErrConflict
	}
	var e definition.Envelope
	if json.Unmarshal(raw, &e) != nil {
		return res, ErrInvalid
	}
	if res.Action == "confirm" {
		descriptor, ok := s.Catalog.Find(e.NodeType, e.NodeVersion)
		if !ok || definition.ValidateSchema(descriptor.OutputSchema, output) != nil {
			return res, ErrInvalid
		}
		res.OutputRef, err = s.PutPayload(ctx, res.WorkspaceID, res.RunID, output)
		if err != nil {
			return res, err
		}
	} else {
		res.OutputRef = ""
		if output != nil {
			return res, ErrInvalid
		}
	}
	tag, err := tx.Exec(ctx, `INSERT INTO operator_resolutions(workspace_id,run_id,invocation_id,action,output_ref,actor,note) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7) ON CONFLICT DO NOTHING`, res.WorkspaceID, res.RunID, res.InvocationID, res.Action, res.OutputRef, res.Actor, res.Note)
	if err != nil {
		return res, err
	}
	if tag.RowsAffected() == 0 {
		var equal bool
		err = tx.QueryRow(ctx, "SELECT action=$4 AND coalesce(output_ref,'')=$5 AND actor=$6 AND note=$7 FROM operator_resolutions WHERE workspace_id=$1 AND run_id=$2 AND invocation_id=$3", res.WorkspaceID, res.RunID, res.InvocationID, res.Action, res.OutputRef, res.Actor, res.Note).Scan(&equal)
		if err != nil {
			return res, err
		}
		if !equal {
			return res, ErrConflict
		}
	}
	return res, tx.Commit(ctx)
}
func (s *Store) Resolution(ctx context.Context, e definition.Envelope) (Resolution, error) {
	res := Resolution{WorkspaceID: e.WorkspaceID, RunID: e.RunID, InvocationID: e.InvocationID}
	err := s.Pool.QueryRow(ctx, "SELECT action,coalesce(output_ref,''),actor,note FROM operator_resolutions WHERE workspace_id=$1 AND run_id=$2 AND invocation_id=$3", e.WorkspaceID, e.RunID, e.InvocationID).Scan(&res.Action, &res.OutputRef, &res.Actor, &res.Note)
	return res, notFound(err)
}
func (s *Store) AcceptResolution(ctx context.Context, e definition.Envelope) (Resolution, error) {
	res, err := s.Resolution(ctx, e)
	if err != nil {
		return res, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)
	if res.Action == "confirm" {
		tag, err := tx.Exec(ctx, "UPDATE node_invocations SET accepted_attempt=0,accepted_output_ref=$4 WHERE workspace_id=$1 AND run_id=$2 AND id=$3 AND (accepted_attempt IS NULL OR accepted_attempt=0)", e.WorkspaceID, e.RunID, e.InvocationID, res.OutputRef)
		if err != nil {
			return res, err
		}
		if tag.RowsAffected() != 1 {
			return res, ErrConflict
		}
	}
	_, err = tx.Exec(ctx, "UPDATE uncertain_invocations SET resolved_at=coalesce(resolved_at,now()) WHERE workspace_id=$1 AND run_id=$2 AND invocation_id=$3", e.WorkspaceID, e.RunID, e.InvocationID)
	if err != nil {
		return res, err
	}
	return res, tx.Commit(ctx)
}
func (s *Store) PendingResolutions(ctx context.Context) ([]Resolution, error) {
	rows, err := s.Pool.Query(ctx, "SELECT d.workspace_id,d.run_id,d.invocation_id FROM operator_resolutions d JOIN uncertain_invocations u USING(workspace_id,run_id,invocation_id) JOIN runs r ON (r.workspace_id,r.id)=(d.workspace_id,d.run_id) WHERE u.resolved_at IS NULL AND r.state NOT IN ('succeeded','failed','cancelled') ORDER BY d.created_at LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Resolution{}
	for rows.Next() {
		var r Resolution
		if err = rows.Scan(&r.WorkspaceID, &r.RunID, &r.InvocationID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
