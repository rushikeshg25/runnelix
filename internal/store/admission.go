package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rushikeshg25/runnelix/internal/definition"
)

type Run struct {
	RetentionHold     bool       `json:"retentionHold"`
	PayloadsExpiredAt *time.Time `json:"payloadsExpiredAt"`
	HistoryExpiredAt  *time.Time `json:"historyExpiredAt"`
	ID                string     `json:"runId"`
	WorkflowID        string     `json:"workflowId"`
	VersionID         string     `json:"workflowVersionId"`
	State             string     `json:"state"`
	AcceptedAt        time.Time  `json:"acceptedAt"`
	RuntimeID         *string    `json:"runtimeId"`
	DispatchState     string     `json:"dispatchState"`
	ExecutionMode     string     `json:"executionMode"`
	CancelRequested   bool       `json:"cancelRequested"`
	Checkpoint        int64      `json:"checkpoint"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

func (s *Store) Admit(ctx context.Context, workspace, workflow, key string, input []byte) (Run, error) {
	return s.admit(ctx, workspace, workflow, key, input, "admission", "manual")
}
func (s *Store) AdmitExecution(ctx context.Context, workspace, workflow, key string, input []byte) (Run, error) {
	return s.admit(ctx, workspace, workflow, key, input, "execute", "manual")
}
func (s *Store) admit(ctx context.Context, workspace, workflow, key string, input []byte, mode, source string) (Run, error) {
	if !validID(key) {
		return Run{}, ErrInvalid
	}
	var value any
	if err := definition.Decode(input, &value); err != nil {
		return Run{}, ErrInvalid
	}
	if _, ok := value.(map[string]any); !ok {
		return Run{}, ErrInvalid
	}
	// Canonical JSON ignores object-key order/whitespace. The key's scope includes workflow and source.
	canonical, err := json.Marshal(value)
	if err != nil {
		return Run{}, ErrInvalid
	}
	if len(canonical) > 8192 {
		return Run{}, ErrInvalid
	}
	sum := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(sum[:])
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	run, err := s.admitTx(ctx, tx, workspace, workflow, key, canonical, fingerprint, mode, source)
	if err != nil {
		return Run{}, err
	}
	return run, tx.Commit(ctx)
}
func (s *Store) admitTx(ctx context.Context, tx pgx.Tx, workspace, workflow, key string, canonical []byte, fingerprint, mode, source string) (Run, error) {
	var err error
	var workspaceLock string
	if err = tx.QueryRow(ctx, "SELECT id FROM workspaces WHERE id=$1 FOR UPDATE", workspace).Scan(&workspaceLock); err != nil {
		return Run{}, notFound(err)
	}
	var active *string
	if err = tx.QueryRow(ctx, "SELECT active_version_id FROM workflows WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspace, workflow).Scan(&active); err != nil {
		return Run{}, notFound(err)
	}
	var existing, stored string
	err = tx.QueryRow(ctx, "SELECT run_id,fingerprint FROM trigger_inbox WHERE workspace_id=$1 AND workflow_id=$2 AND source=$4 AND trigger_key=$3", workspace, workflow, key, source).Scan(&existing, &stored)
	if err == nil {
		if stored != fingerprint {
			return Run{}, ErrConflict
		}
		run, e := getRun(ctx, tx, workspace, existing)
		if e != nil {
			return Run{}, e
		}
		if run.ExecutionMode != mode {
			return Run{}, ErrConflict
		}
		return run, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Run{}, err
	}
	var scheduledWorkflow bool
	if mode == "execute" {
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schedules WHERE workspace_id=$1 AND workflow_id=$2)", workspace, workflow).Scan(&scheduledWorkflow); err != nil {
			return Run{}, err
		}
	}
	if mode == "execute" && (source == "schedule" || scheduledWorkflow) {
		var activeCount int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM runs WHERE workspace_id=$1 AND workflow_id=$2 AND execution_mode='execute' AND state NOT IN ('succeeded','failed','cancelled')", workspace, workflow).Scan(&activeCount); err != nil {
			return Run{}, err
		}
		if activeCount > 0 {
			return Run{}, ErrOverlap
		}
	}
	if mode == "execute" {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM runs WHERE workspace_id=$1 AND execution_mode='execute' AND state NOT IN ('succeeded','failed','cancelled')", workspace).Scan(&count); err != nil {
			return Run{}, err
		}
		if count >= 100 {
			return Run{}, ErrLimit
		}
	}
	if active == nil {
		return Run{}, ErrUnpublished
	}
	// Recheck catalog compatibility before admission. A stale deployment must not admit an unexecutable version.
	version, err := getVersion(ctx, tx, workspace, workflow, *active)
	if err != nil {
		return Run{}, err
	}
	_, _, digest, err := definition.Compile(version.Graph, s.Catalog)
	if err != nil {
		return Run{}, err
	}
	if digest != version.Digest {
		return Run{}, ErrConflict
	}
	id := "run_" + uuid.NewString()
	if _, err = tx.Exec(ctx, "INSERT INTO runs(workspace_id,id,workflow_id,version_id,input,execution_mode) VALUES($1,$2,$3,$4,$5,$6)", workspace, id, workflow, *active, canonical, mode); err != nil {
		return Run{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO trigger_inbox(workspace_id,workflow_id,source,trigger_key,fingerprint,run_id) VALUES($1,$2,$6,$3,$4,$5)", workspace, workflow, key, fingerprint, id, source); err != nil {
		return Run{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO start_outbox(workspace_id,run_id) VALUES($1,$2)", workspace, id); err != nil {
		return Run{}, err
	}
	run, err := getRun(ctx, tx, workspace, id)
	if err != nil {
		return Run{}, err
	}
	if err = appendAudit(ctx, tx, workspace, "run.admitted", id, AuditDetails{WorkflowID: workflow, VersionID: run.VersionID, Mode: mode, Source: source}); err != nil {
		return Run{}, err
	}
	return run, nil
}

const runSelect = `SELECT r.id,r.workflow_id,r.version_id,r.state,r.accepted_at,receipt.runtime_id,
 CASE WHEN o.dispatched_at IS NOT NULL THEN 'acknowledged' WHEN receipt.run_id IS NOT NULL THEN 'receipt_recorded' ELSE 'pending' END, r.execution_mode,r.cancel_requested,r.checkpoint,r.updated_at,r.retention_hold,r.payloads_expired_at,r.history_expired_at
 FROM runs r JOIN start_outbox o ON (o.workspace_id,o.run_id)=(r.workspace_id,r.id)
 LEFT JOIN runtime_receipts receipt ON (receipt.workspace_id,receipt.run_id)=(r.workspace_id,r.id)`

func scanRun(row pgx.Row) (Run, error) {
	var run Run
	err := row.Scan(&run.ID, &run.WorkflowID, &run.VersionID, &run.State, &run.AcceptedAt, &run.RuntimeID, &run.DispatchState, &run.ExecutionMode, &run.CancelRequested, &run.Checkpoint, &run.UpdatedAt, &run.RetentionHold, &run.PayloadsExpiredAt, &run.HistoryExpiredAt)
	return run, notFound(err)
}
func getRun(ctx context.Context, q querier, workspace, id string) (Run, error) {
	return scanRun(q.QueryRow(ctx, runSelect+" WHERE r.workspace_id=$1 AND r.id=$2", workspace, id))
}
func (s *Store) GetRun(ctx context.Context, workspace, id string) (Run, error) {
	run, err := getRun(ctx, s.Pool, workspace, id)
	if err == nil && run.HistoryExpiredAt != nil {
		return Run{}, ErrExpired
	}
	return run, err
}

func (s *Store) AdmitWebhook(ctx context.Context, workspace, workflow, key string, input []byte) (Run, error) {
	return s.admit(ctx, workspace, workflow, key, input, "execute", "webhook")
}
