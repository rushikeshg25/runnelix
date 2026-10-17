package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type StartRequest struct {
	WorkspaceID string `json:"workspaceId"`
	RunID       string `json:"runId"`
	VersionID   string `json:"versionId"`
	PlanDigest  string `json:"planDigest"`
}
type Claim struct {
	StartRequest
	Token    string
	Attempts int
}

func RuntimeID(run string) string { return "runnelix/admission/" + run }
func (s *Store) Claim(ctx context.Context, lease time.Duration) (Claim, error) {
	if lease < time.Second || lease > 5*time.Minute {
		return Claim{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Claim{}, err
	}
	defer tx.Rollback(ctx)
	var c Claim
	c.Token = uuid.NewString()
	err = tx.QueryRow(ctx, `WITH candidate AS (
 SELECT run_id FROM start_outbox WHERE dispatched_at IS NULL AND next_try<=now() AND (lease_until IS NULL OR lease_until<=now())
 ORDER BY next_try,run_id FOR UPDATE SKIP LOCKED LIMIT 1)
 UPDATE start_outbox o SET lease_token=$1,lease_until=now()+($2*interval '1 millisecond'),attempts=attempts+1 FROM candidate c WHERE o.run_id=c.run_id RETURNING o.workspace_id,o.run_id,o.attempts`, c.Token, lease.Milliseconds()).Scan(&c.WorkspaceID, &c.RunID, &c.Attempts)
	if err != nil {
		return Claim{}, notFound(err)
	}
	err = tx.QueryRow(ctx, `SELECT r.version_id,v.digest FROM runs r JOIN workflow_versions v ON (v.workspace_id,v.workflow_id,v.id)=(r.workspace_id,r.workflow_id,r.version_id) WHERE r.workspace_id=$1 AND r.id=$2`, c.WorkspaceID, c.RunID).Scan(&c.VersionID, &c.PlanDigest)
	if err != nil {
		return Claim{}, err
	}
	return c, tx.Commit(ctx)
}
func (s *Store) HasReceipt(ctx context.Context, r StartRequest) (bool, error) {
	var present bool
	err := s.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM runtime_receipts WHERE workspace_id=$1 AND run_id=$2 AND runtime_id=$3)", r.WorkspaceID, r.RunID, RuntimeID(r.RunID)).Scan(&present)
	return present, err
}
func (s *Store) RecordReceipt(ctx context.Context, r StartRequest, runtimeID string) error {
	if runtimeID != RuntimeID(r.RunID) {
		return ErrConflict
	}
	// Verify the pinned version and digest again inside the runtime's idempotent activity.
	tag, err := s.Pool.Exec(ctx, `INSERT INTO runtime_receipts(workspace_id,run_id,runtime_id)
 SELECT r.workspace_id,r.id,$5 FROM runs r JOIN workflow_versions v ON (v.workspace_id,v.workflow_id,v.id)=(r.workspace_id,r.workflow_id,r.version_id)
 WHERE r.workspace_id=$1 AND r.id=$2 AND r.version_id=$3 AND v.digest=$4 ON CONFLICT(run_id) DO NOTHING`, r.WorkspaceID, r.RunID, r.VersionID, r.PlanDigest, runtimeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var valid bool
	err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runtime_receipts p JOIN runs r ON (r.workspace_id,r.id)=(p.workspace_id,p.run_id) JOIN workflow_versions v ON (v.workspace_id,v.workflow_id,v.id)=(r.workspace_id,r.workflow_id,r.version_id) WHERE p.workspace_id=$1 AND p.run_id=$2 AND r.version_id=$3 AND v.digest=$4 AND p.runtime_id=$5)`, r.WorkspaceID, r.RunID, r.VersionID, r.PlanDigest, runtimeID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrConflict
	}
	return nil
}
func (s *Store) Acknowledge(ctx context.Context, c Claim) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE start_outbox SET dispatched_at=now(),lease_token=NULL,lease_until=NULL WHERE workspace_id=$1 AND run_id=$2 AND lease_token=$3 AND lease_until>now() AND EXISTS(SELECT 1 FROM runtime_receipts WHERE workspace_id=$1 AND run_id=$2)`, c.WorkspaceID, c.RunID, c.Token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func (s *Store) Retry(ctx context.Context, c Claim, delay time.Duration) error {
	if delay < 0 || delay > time.Minute {
		return ErrInvalid
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE start_outbox SET next_try=now()+($4*interval '1 millisecond'),lease_token=NULL,lease_until=NULL WHERE workspace_id=$1 AND run_id=$2 AND lease_token=$3 AND lease_until>now()`, c.WorkspaceID, c.RunID, c.Token, delay.Milliseconds())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func IsEmpty(err error) bool { return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows) }
