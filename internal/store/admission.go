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
	ID            string    `json:"runId"`
	WorkflowID    string    `json:"workflowId"`
	VersionID     string    `json:"workflowVersionId"`
	State         string    `json:"state"`
	AcceptedAt    time.Time `json:"acceptedAt"`
	RuntimeID     *string   `json:"runtimeId"`
	DispatchState string    `json:"dispatchState"`
}

func (s *Store) Admit(ctx context.Context, workspace, workflow, key string, input []byte) (Run, error) {
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
	var active *string
	if err = tx.QueryRow(ctx, "SELECT active_version_id FROM workflows WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspace, workflow).Scan(&active); err != nil {
		return Run{}, notFound(err)
	}
	var existing, stored string
	err = tx.QueryRow(ctx, "SELECT run_id,fingerprint FROM trigger_inbox WHERE workspace_id=$1 AND workflow_id=$2 AND source='manual' AND trigger_key=$3", workspace, workflow, key).Scan(&existing, &stored)
	if err == nil {
		if stored != fingerprint {
			return Run{}, ErrConflict
		}
		run, e := getRun(ctx, tx, workspace, existing)
		if e != nil {
			return Run{}, e
		}
		return run, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Run{}, err
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
	if _, err = tx.Exec(ctx, "INSERT INTO runs(workspace_id,id,workflow_id,version_id,input) VALUES($1,$2,$3,$4,$5)", workspace, id, workflow, *active, canonical); err != nil {
		return Run{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO trigger_inbox(workspace_id,workflow_id,source,trigger_key,fingerprint,run_id) VALUES($1,$2,'manual',$3,$4,$5)", workspace, workflow, key, fingerprint, id); err != nil {
		return Run{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO start_outbox(workspace_id,run_id) VALUES($1,$2)", workspace, id); err != nil {
		return Run{}, err
	}
	run, err := getRun(ctx, tx, workspace, id)
	if err != nil {
		return Run{}, err
	}
	return run, tx.Commit(ctx)
}
func getRun(ctx context.Context, q querier, workspace, id string) (Run, error) {
	var run Run
	err := q.QueryRow(ctx, `SELECT r.id,r.workflow_id,r.version_id,r.state,r.accepted_at,receipt.runtime_id,
 CASE WHEN o.dispatched_at IS NOT NULL THEN 'acknowledged' WHEN receipt.run_id IS NOT NULL THEN 'receipt_recorded' ELSE 'pending' END
 FROM runs r JOIN start_outbox o ON (o.workspace_id,o.run_id)=(r.workspace_id,r.id)
 LEFT JOIN runtime_receipts receipt ON (receipt.workspace_id,receipt.run_id)=(r.workspace_id,r.id)
 WHERE r.workspace_id=$1 AND r.id=$2`, workspace, id).Scan(&run.ID, &run.WorkflowID, &run.VersionID, &run.State, &run.AcceptedAt, &run.RuntimeID, &run.DispatchState)
	return run, notFound(err)
}
func (s *Store) GetRun(ctx context.Context, workspace, id string) (Run, error) {
	return getRun(ctx, s.Pool, workspace, id)
}
