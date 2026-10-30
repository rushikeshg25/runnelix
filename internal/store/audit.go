package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rushikeshg25/runnelix/internal/identity"
)

type auditContextKey struct{}
type auditContext struct{ Actor, RequestID string }

// WithAuditContext is called only by trusted request/CLI boundaries, never from
// client-supplied actor or request-ID headers. Background work defaults to system.
func WithAuditContext(ctx context.Context, actor, requestID string) context.Context {
	return context.WithValue(ctx, auditContextKey{}, auditContext{actor, requestID})
}
func PrincipalActor(p identity.Principal) string {
	raw, _ := json.Marshal(p)
	return "oidc:" + TokenHash(string(raw))
}

type AuditGrants struct {
	Publish  bool `json:"publish"`
	Execute  bool `json:"execute"`
	DataRead bool `json:"dataRead"`
}

// Deliberately no arbitrary metadata map: credentials, graph config, inputs,
// notes, destinations, session values and provider responses cannot be captured.
type AuditDetails struct {
	RunID        string       `json:"runId,omitempty"`
	WorkflowID   string       `json:"workflowId,omitempty"`
	VersionID    string       `json:"versionId,omitempty"`
	Revision     int64        `json:"revision,omitempty"`
	Role         string       `json:"role,omitempty"`
	PreviousRole string       `json:"previousRole,omitempty"`
	Grants       *AuditGrants `json:"grants,omitempty"`
	Enabled      *bool        `json:"enabled,omitempty"`
	Mode         string       `json:"mode,omitempty"`
	Source       string       `json:"source,omitempty"`
	Decision     string       `json:"decision,omitempty"`
}
type AuditRecord struct {
	Sequence   int64        `json:"sequence"`
	Actor      string       `json:"actor"`
	RequestID  string       `json:"requestId"`
	Action     string       `json:"action"`
	TargetID   string       `json:"targetId"`
	Details    AuditDetails `json:"details"`
	RecordedAt time.Time    `json:"recordedAt"`
}

// Append at the end of the business transaction, after its other locks/writes.
// Updating the workspace counter serializes audit commits and prevents a reader
// from advancing its cursor past an uncommitted lower sequence.
func appendAudit(ctx context.Context, tx pgx.Tx, workspace, action, target string, details AuditDetails) error {
	actor := auditContext{Actor: "system"}
	if value, ok := ctx.Value(auditContextKey{}).(auditContext); ok {
		actor = value
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	var sequence int64
	err = tx.QueryRow(ctx, `INSERT INTO audit_cursors(workspace_id,sequence) VALUES($1,1) ON CONFLICT(workspace_id) DO UPDATE SET sequence=audit_cursors.sequence+1 RETURNING sequence`, workspace).Scan(&sequence)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_records(workspace_id,sequence,actor,request_id,action,target_id,details) VALUES($1,$2,$3,$4,$5,$6,$7)`, workspace, sequence, actor.Actor, actor.RequestID, action, target, raw)
	return err
}
func (s *Store) AuditRecords(ctx context.Context, workspace string, after int64) ([]AuditRecord, error) {
	if after < 0 || after > 9007199254740991 {
		return nil, ErrInvalid
	}
	rows, err := s.Pool.Query(ctx, `SELECT sequence,actor,request_id,action,target_id,details,recorded_at FROM audit_records WHERE workspace_id=$1 AND sequence>$2 ORDER BY sequence LIMIT 100`, workspace, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []AuditRecord{}
	for rows.Next() {
		var record AuditRecord
		var raw []byte
		if err = rows.Scan(&record.Sequence, &record.Actor, &record.RequestID, &record.Action, &record.TargetID, &raw, &record.RecordedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &record.Details); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func auditRequestID(ctx context.Context) string {
	if value, ok := ctx.Value(auditContextKey{}).(auditContext); ok {
		return value.RequestID
	}
	return ""
}
