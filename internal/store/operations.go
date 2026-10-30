package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// CheckSchema is read-only. Readiness must not silently migrate an installation.
func (s *Store) CheckSchema(ctx context.Context) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	rows, err := s.Pool.Query(ctx, "SELECT version,checksum FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("schema unavailable")
	}
	defer rows.Close()
	found := map[string]string{}
	for rows.Next() {
		var version, checksum string
		if err = rows.Scan(&version, &checksum); err != nil {
			return err
		}
		found[version] = checksum
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(found) != len(entries) {
		return fmt.Errorf("schema version mismatch")
	}
	for _, e := range entries {
		raw, err := migrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		if found[e.Name()] != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("schema checksum mismatch")
		}
	}
	return nil
}

type OperationalMetrics struct {
	Runs                  map[string]int64 `json:"runs"`
	PendingStarts         int64            `json:"pendingStarts"`
	OldestPendingSeconds  float64          `json:"oldestPendingSeconds"`
	UnresolvedInvocations int64            `json:"unresolvedInvocations"`
	RetentionHolds        int64            `json:"retentionHolds"`
	PayloadBytes          int64            `json:"payloadBytes"`
	ExpiredHistories      int64            `json:"expiredHistories"`
}

// Fixed keys and aggregate values only: no identities, payloads, destinations,
// or run IDs. The caller supplies a short deadline to bound database work.
func (s *Store) OperationalMetrics(ctx context.Context, workspace string) (OperationalMetrics, error) {
	m := OperationalMetrics{Runs: map[string]int64{"accepted": 0, "running": 0, "waiting": 0, "needs_attention": 0, "succeeded": 0, "failed": 0, "cancelled": 0}}
	rows, err := s.Pool.Query(ctx, "SELECT state,count(*) FROM runs WHERE workspace_id=$1 AND history_expired_at IS NULL GROUP BY state", workspace)
	if err != nil {
		return m, err
	}
	for rows.Next() {
		var state string
		var count int64
		if err = rows.Scan(&state, &count); err != nil {
			rows.Close()
			return m, err
		}
		m.Runs[state] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return m, err
	}
	err = s.Pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM start_outbox WHERE workspace_id=$1 AND dispatched_at IS NULL),
 (SELECT coalesce(greatest(extract(epoch FROM now()-min(r.accepted_at)),0),0)::float8 FROM start_outbox o JOIN runs r ON (r.workspace_id,r.id)=(o.workspace_id,o.run_id) WHERE o.workspace_id=$1 AND o.dispatched_at IS NULL),
 (SELECT count(*) FROM uncertain_invocations WHERE workspace_id=$1 AND resolved_at IS NULL),
 (SELECT count(*) FROM runs WHERE workspace_id=$1 AND retention_hold),
 (SELECT coalesce(sum(payload_bytes),0) FROM runs WHERE workspace_id=$1 AND payloads_expired_at IS NULL),
 (SELECT count(*) FROM runs WHERE workspace_id=$1 AND history_expired_at IS NOT NULL)`, workspace).Scan(&m.PendingStarts, &m.OldestPendingSeconds, &m.UnresolvedInvocations, &m.RetentionHolds, &m.PayloadBytes, &m.ExpiredHistories)
	return m, err
}
