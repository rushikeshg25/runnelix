package store

import (
	"context"
	"encoding/json"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"sort"
)

func (s *Store) ProjectionCandidate(ctx context.Context) (StartRequest, int64, error) {
	var r StartRequest
	var checkpoint int64
	err := s.Pool.QueryRow(ctx, `WITH candidate AS (
 SELECT r.workspace_id,r.id FROM runs r JOIN workflow_versions v ON (v.workspace_id,v.workflow_id,v.id)=(r.workspace_id,r.workflow_id,r.version_id)
 WHERE r.execution_mode='execute' AND r.state NOT IN ('succeeded','failed','cancelled') AND (v.plan->>'interpreterVersion')::int=3 ORDER BY r.projection_checked_at,r.id FOR UPDATE OF r SKIP LOCKED LIMIT 1)
 UPDATE runs r SET projection_checked_at=now() FROM candidate c,workflow_versions v WHERE (r.workspace_id,r.id)=(c.workspace_id,c.id) AND (v.workspace_id,v.workflow_id,v.id)=(r.workspace_id,r.workflow_id,r.version_id)
 RETURNING r.workspace_id,r.id,r.version_id,v.digest,r.checkpoint`).Scan(&r.WorkspaceID, &r.RunID, &r.VersionID, &r.PlanDigest, &checkpoint)
	r.ExecutionMode = "execute"
	r.InterpreterVersion = 3
	return r, checkpoint, notFound(err)
}

type ArtifactReference struct {
	Digest   string `json:"digest"`
	Versions int    `json:"versions"`
}

func (s *Store) RequiredArtifacts(ctx context.Context, workspace string) ([]ArtifactReference, error) {
	rows, err := s.Pool.Query(ctx, `SELECT v.plan FROM workflow_versions v WHERE v.workspace_id=$1 AND (EXISTS(SELECT 1 FROM workflows w WHERE (w.workspace_id,w.active_version_id)=(v.workspace_id,v.id)) OR EXISTS(SELECT 1 FROM runs r WHERE (r.workspace_id,r.version_id)=(v.workspace_id,v.id) AND r.execution_mode='execute' AND r.state NOT IN ('succeeded','failed','cancelled')))`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var plan definition.Plan
		if err = json.Unmarshal(raw, &plan); err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, step := range plan.Steps {
			seen[step.ImplementationDigest] = true
			if step.Body != nil {
				seen[step.Body.ImplementationDigest] = true
			}
		}
		for digest := range seen {
			counts[digest]++
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	result := []ArtifactReference{}
	for digest, count := range counts {
		result = append(result, ArtifactReference{digest, count})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Digest < result[j].Digest })
	return result, nil
}
