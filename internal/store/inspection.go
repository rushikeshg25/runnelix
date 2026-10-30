package store

import (
	"context"
)

// Lists use a stable ID cursor and a fixed page size to bound database work.
func (s *Store) Workflows(ctx context.Context, workspace, after string) ([]Workflow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT w.id,w.name,d.revision,d.graph,w.active_version_id FROM workflows w JOIN workflow_drafts d ON (d.workspace_id,d.workflow_id)=(w.workspace_id,w.id) WHERE w.workspace_id=$1 AND w.id>$2 ORDER BY w.id LIMIT 50`, workspace, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Workflow{}
	for rows.Next() {
		var w Workflow
		if err = rows.Scan(&w.ID, &w.Name, &w.Revision, &w.Graph, &w.ActiveVersionID); err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}
func (s *Store) Runs(ctx context.Context, workspace, workflow, after string) ([]Run, error) {
	rows, err := s.Pool.Query(ctx, runSelect+" WHERE r.workspace_id=$1 AND r.workflow_id=$2 AND r.id>$3 AND r.history_expired_at IS NULL ORDER BY r.id LIMIT 50", workspace, workflow, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func (s *Store) PendingCancellations(ctx context.Context) ([]StartRequest, error) {
	rows, err := s.Pool.Query(ctx, "SELECT workspace_id,id FROM runs WHERE execution_mode='execute' AND cancel_requested AND state NOT IN ('succeeded','failed','cancelled') ORDER BY updated_at,id LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []StartRequest{}
	for rows.Next() {
		r := StartRequest{ExecutionMode: "execute"}
		if err = rows.Scan(&r.WorkspaceID, &r.RunID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
