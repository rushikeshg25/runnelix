package store

import (
	"context"
	"time"
)

// Retention is opt-in and bounded. Days are elapsed 24-hour periods after the
// terminal projection, not time since admission (which may include long waits).
type RetentionPolicy struct {
	PayloadDays int
	HistoryDays int
	Limit       int
}
type RetentionCandidate struct {
	RunID         string `json:"runId"`
	ExpireHistory bool   `json:"expireHistory"`
}
type RetentionResult struct {
	Applied    bool                 `json:"applied"`
	Candidates []RetentionCandidate `json:"candidates"`
}

func (s *Store) RetainTerminalData(ctx context.Context, workspace string, policy RetentionPolicy, apply bool) (RetentionResult, error) {
	result := RetentionResult{Applied: apply, Candidates: []RetentionCandidate{}}
	if workspace == "" || policy.PayloadDays < 1 || policy.HistoryDays < policy.PayloadDays || policy.HistoryDays > 36500 || policy.Limit < 1 || policy.Limit > 100 {
		return result, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM workspaces WHERE id=$1)", workspace).Scan(&exists); err != nil {
		return result, err
	}
	if !exists {
		return result, ErrNotFound
	}
	// Lock even previews so reported candidates obey the same selection rules.
	// SKIP LOCKED lets concurrent batches and active writers proceed safely.
	rows, err := tx.Query(ctx, `SELECT r.id,r.updated_at < now()-($3*interval '24 hours')
 FROM runs r WHERE r.workspace_id=$1 AND r.state IN ('succeeded','failed','cancelled')
 AND NOT r.retention_hold AND r.history_expired_at IS NULL
 AND ((r.payloads_expired_at IS NULL AND r.updated_at < now()-($2*interval '24 hours')) OR r.updated_at < now()-($3*interval '24 hours'))
 AND EXISTS(SELECT 1 FROM start_outbox o WHERE o.workspace_id=r.workspace_id AND o.run_id=r.id AND o.dispatched_at IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM uncertain_invocations u WHERE u.workspace_id=r.workspace_id AND u.run_id=r.id AND u.resolved_at IS NULL)
 ORDER BY r.updated_at,r.id LIMIT $4 FOR UPDATE OF r SKIP LOCKED`, workspace, policy.PayloadDays, policy.HistoryDays, policy.Limit)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var c RetentionCandidate
		if err = rows.Scan(&c.RunID, &c.ExpireHistory); err != nil {
			rows.Close()
			return result, err
		}
		result.Candidates = append(result.Candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	// Child rows may commit between the candidate statement's snapshot and
	// acquisition of its parent lock. Recheck in a fresh READ COMMITTED statement.
	// New child inserts now block on our FOR UPDATE lock and reject expiry.
	eligible := result.Candidates[:0]
	for _, candidate := range result.Candidates {
		var unresolved bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM uncertain_invocations WHERE workspace_id=$1 AND run_id=$2 AND resolved_at IS NULL)", workspace, candidate.RunID).Scan(&unresolved); err != nil {
			return result, err
		}
		if !unresolved {
			eligible = append(eligible, candidate)
		}
	}
	result.Candidates = eligible
	if !apply {
		return result, nil
	}
	for _, c := range result.Candidates {
		tag, err := tx.Exec(ctx, `UPDATE runs SET input='null'::jsonb,payloads_expired_at=now() WHERE workspace_id=$1 AND id=$2 AND payloads_expired_at IS NULL`, workspace, c.RunID)
		if err != nil {
			return result, err
		}
		if tag.RowsAffected() > 0 {
			if _, err = tx.Exec(ctx, `UPDATE payloads SET data=NULL,expired_at=now() WHERE workspace_id=$1 AND run_id=$2 AND expired_at IS NULL`, workspace, c.RunID); err != nil {
				return result, err
			}
			if err = appendAudit(ctx, tx, workspace, "run.payloads_expired", c.RunID, AuditDetails{}); err != nil {
				return result, err
			}
		}
		if c.ExpireHistory {
			if _, err = tx.Exec(ctx, `UPDATE runs SET history_expired_at=now() WHERE workspace_id=$1 AND id=$2`, workspace, c.RunID); err != nil {
				return result, err
			}
			// Fixed table names and dependency order. Preserve the run receipt and all
			// inbox/outbox/runtime/schedule identities to prevent duplicate execution.
			for _, table := range []string{"operator_resolutions", "uncertain_invocations", "attempts", "node_invocations", "run_events", "payloads"} {
				if _, err = tx.Exec(ctx, "DELETE FROM "+table+" WHERE workspace_id=$1 AND run_id=$2", workspace, c.RunID); err != nil {
					return result, err
				}
			}
			if err = appendAudit(ctx, tx, workspace, "run.history_expired", c.RunID, AuditDetails{}); err != nil {
				return result, err
			}
		}
	}
	return result, tx.Commit(ctx)
}

// Holds prevent future expiry; they cannot recover already expired data.
func (s *Store) SetRetentionHold(ctx context.Context, workspace, run string, hold bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var old bool
	var expired *time.Time
	if err = tx.QueryRow(ctx, "SELECT retention_hold,history_expired_at FROM runs WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspace, run).Scan(&old, &expired); err != nil {
		return notFound(err)
	}
	if expired != nil {
		return ErrExpired
	}
	if old == hold {
		return nil
	}
	if _, err = tx.Exec(ctx, "UPDATE runs SET retention_hold=$3 WHERE workspace_id=$1 AND id=$2", workspace, run, hold); err != nil {
		return err
	}
	if err = appendAudit(ctx, tx, workspace, "run.retention_hold_set", run, AuditDetails{Enabled: &hold}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
