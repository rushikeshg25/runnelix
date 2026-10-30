package store

import (
	"context"
	"github.com/google/uuid"
	"github.com/rushikeshg25/runnelix/internal/definition"
)

func (s *Store) ImportWorkflow(ctx context.Context, workspace string, bundle definition.PortableWorkflow, bindings map[string]string) (Workflow, error) {
	graph, err := definition.BindPortable(bundle, bindings, s.Catalog)
	if err != nil {
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
	// Share locks serialize import with rotation/revocation until the new grants commit.
	for _, slot := range bundle.CredentialSlots {
		var id string
		err = tx.QueryRow(ctx, "SELECT id FROM credentials WHERE workspace_id=$1 AND id=$2 AND kind=$3 AND destination=$4 AND NOT revoked FOR SHARE", workspace, bindings[slot.Slot], slot.Kind, slot.Destination).Scan(&id)
		if err != nil {
			return Workflow{}, notFound(err)
		}
	}
	id := "workflow_" + uuid.NewString()
	if _, err = tx.Exec(ctx, "INSERT INTO workflows(workspace_id,id,name) VALUES($1,$2,$3)", workspace, id, bundle.Name); err != nil {
		return Workflow{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO workflow_drafts(workspace_id,workflow_id,graph) VALUES($1,$2,$3)", workspace, id, graph); err != nil {
		return Workflow{}, err
	}
	for _, slot := range bundle.CredentialSlots {
		if _, err = tx.Exec(ctx, "INSERT INTO credential_grants(workspace_id,credential_id,workflow_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", workspace, bindings[slot.Slot], id); err != nil {
			return Workflow{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Workflow{}, err
	}
	return Workflow{ID: id, Name: bundle.Name, Revision: 1, Graph: graph}, nil
}
