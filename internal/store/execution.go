package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/rushikeshg25/runnelix/internal/definition"
)

type Snapshot struct {
	Plan            definition.Plan `json:"plan"`
	InputRef        string          `json:"inputRef"`
	CancelRequested bool            `json:"cancelRequested"`
}
type Event struct {
	InvocationID string `json:"invocationId,omitempty"`
	Sequence     int64  `json:"sequence"`
	State        string `json:"state"`
	NodeID       string `json:"nodeId,omitempty"`
	NodeState    string `json:"nodeState,omitempty"`
	OutputRef    string `json:"outputRef,omitempty"`
	Port         string `json:"port,omitempty"`
	ErrorCode    string `json:"errorCode,omitempty"`
}
type RecordedEvent struct {
	Event
	RecordedAt time.Time `json:"recordedAt"`
}
type Attempt struct {
	InvocationID string    `json:"invocationId"`
	Attempt      int       `json:"attempt"`
	Status       string    `json:"status"`
	OutputRef    *string   `json:"outputRef"`
	ErrorCode    *string   `json:"errorCode"`
	CompletedAt  time.Time `json:"completedAt"`
}

func (s *Store) ExecutionSnapshot(ctx context.Context, r StartRequest) (Snapshot, error) {
	var result Snapshot
	var plan, input []byte
	var digest, executionMode string
	err := s.Pool.QueryRow(ctx, `SELECT v.plan,v.digest,r.input,r.cancel_requested,r.execution_mode FROM runs r JOIN workflow_versions v ON (v.workspace_id,v.workflow_id,v.id)=(r.workspace_id,r.workflow_id,r.version_id) WHERE r.workspace_id=$1 AND r.id=$2 AND r.version_id=$3`, r.WorkspaceID, r.RunID, r.VersionID).Scan(&plan, &digest, &input, &result.CancelRequested, &executionMode)
	if err != nil {
		return result, notFound(err)
	}
	if executionMode != "execute" || digest != r.PlanDigest {
		return result, ErrConflict
	}
	if err = json.Unmarshal(plan, &result.Plan); err != nil {
		return result, err
	}
	var value any
	if err = json.Unmarshal(input, &value); err != nil {
		return result, err
	}
	result.InputRef, err = s.PutPayload(ctx, r.WorkspaceID, r.RunID, value)
	return result, err
}
func (s *Store) PutPayload(ctx context.Context, workspace, run string, value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > 8192 {
		return "", ErrInvalid
	}
	var parsed any
	if definition.Decode(encoded, &parsed) != nil {
		return "", ErrInvalid
	}
	sum := sha256.Sum256(encoded)
	digest := hex.EncodeToString(sum[:])
	id := "payload_" + digest
	_, err = s.Pool.Exec(ctx, "INSERT INTO payloads(workspace_id,run_id,id,data,digest) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING", workspace, run, id, encoded, digest)
	return id, err
}
func (s *Store) Payload(ctx context.Context, workspace, run, ref string) (json.RawMessage, error) {
	var data json.RawMessage
	err := s.Pool.QueryRow(ctx, "SELECT data FROM payloads WHERE workspace_id=$1 AND run_id=$2 AND id=$3", workspace, run, ref).Scan(&data)
	return data, notFound(err)
}
func (s *Store) PrepareInvocation(ctx context.Context, r StartRequest, step definition.Step, inputRef string) (definition.Envelope, error) {
	return s.PrepareScopedInvocation(ctx, r, step, inputRef, "root")
}
func (s *Store) PrepareScopedInvocation(ctx context.Context, r StartRequest, step definition.Step, inputRef, scope string) (definition.Envelope, error) {
	if !validID(scope) {
		return definition.Envelope{}, ErrInvalid
	}

	if _, err := s.Payload(ctx, r.WorkspaceID, r.RunID, inputRef); err != nil {
		return definition.Envelope{}, err
	}
	configRef, err := s.PutPayload(ctx, r.WorkspaceID, r.RunID, step.Config)
	if err != nil {
		return definition.Envelope{}, err
	}
	envelope := definition.Envelope{ContractVersion: 1, WorkspaceID: r.WorkspaceID, RunID: r.RunID, NodeID: step.ID, InvocationID: "invocation_" + step.ID + mapScope(scope), NodeType: step.Type, NodeVersion: step.Version, ImplementationDigest: step.ImplementationDigest, InputRef: inputRef, ConfigRef: configRef, LogicalActionKey: r.RunID + "/" + step.ID + "/" + scope}
	data, err := json.Marshal(envelope)
	if err != nil {
		return envelope, err
	}
	if _, err = definition.ParseEnvelope(data); err != nil {
		return envelope, ErrInvalid
	}
	_, err = s.Pool.Exec(ctx, "INSERT INTO node_invocations(workspace_id,run_id,id,node_id,envelope) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING", r.WorkspaceID, r.RunID, envelope.InvocationID, step.ID, data)
	if err != nil {
		return envelope, err
	}
	var equal bool
	err = s.Pool.QueryRow(ctx, "SELECT envelope=$4::jsonb FROM node_invocations WHERE workspace_id=$1 AND run_id=$2 AND id=$3", r.WorkspaceID, r.RunID, envelope.InvocationID, data).Scan(&equal)
	if err != nil {
		return envelope, err
	}
	if !equal {
		return envelope, ErrConflict
	}
	return envelope, nil
}
func (s *Store) Invocation(ctx context.Context, e definition.Envelope) error {
	data, err := json.Marshal(e)
	if err != nil {
		return ErrInvalid
	}
	var equal bool
	err = s.Pool.QueryRow(ctx, "SELECT envelope=$4::jsonb FROM node_invocations WHERE workspace_id=$1 AND run_id=$2 AND id=$3", e.WorkspaceID, e.RunID, e.InvocationID, data).Scan(&equal)
	if err != nil {
		return notFound(err)
	}
	if !equal {
		return ErrConflict
	}
	return nil
}

// RecordAttempt retains every immutable completion. It never selects a winner.
func (s *Store) RecordAttempt(ctx context.Context, e definition.Envelope, attempt int, status, outputRef, code string) error {
	if err := s.Invocation(ctx, e); err != nil {
		return err
	}
	if attempt < 1 || attempt > 100 || !(status == "succeeded" || status == "failed" || status == "uncertain") {
		return ErrInvalid
	}
	if code != "" && !validID(code) {
		return ErrInvalid
	}
	if status == "succeeded" && outputRef == "" {
		return ErrInvalid
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO attempts(workspace_id,run_id,invocation_id,attempt,status,output_ref,error_code) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,'')) ON CONFLICT DO NOTHING`, e.WorkspaceID, e.RunID, e.InvocationID, attempt, status, outputRef, code)
	if err != nil {
		return err
	}
	var equal bool
	err = s.Pool.QueryRow(ctx, "SELECT status=$5 AND coalesce(output_ref,'')=$6 AND coalesce(error_code,'')=$7 FROM attempts WHERE workspace_id=$1 AND run_id=$2 AND invocation_id=$3 AND attempt=$4", e.WorkspaceID, e.RunID, e.InvocationID, attempt, status, outputRef, code).Scan(&equal)
	if err != nil {
		return err
	}
	if !equal {
		return ErrConflict
	}
	return nil
}

// AcceptAttempt is called only by the orchestrator after Temporal accepted that attempt.
func (s *Store) AcceptAttempt(ctx context.Context, e definition.Envelope, attempt int) (string, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE node_invocations i SET accepted_attempt=a.attempt,accepted_output_ref=a.output_ref FROM attempts a WHERE (a.workspace_id,a.run_id,a.invocation_id)=(i.workspace_id,i.run_id,i.id) AND i.workspace_id=$1 AND i.run_id=$2 AND i.id=$3 AND a.attempt=$4 AND a.status='succeeded' AND (i.accepted_attempt IS NULL OR i.accepted_attempt=$4)`, e.WorkspaceID, e.RunID, e.InvocationID, attempt)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", ErrConflict
	}
	var ref string
	err = s.Pool.QueryRow(ctx, "SELECT accepted_output_ref FROM node_invocations WHERE workspace_id=$1 AND run_id=$2 AND id=$3", e.WorkspaceID, e.RunID, e.InvocationID).Scan(&ref)
	return ref, err
}
func (s *Store) ApplyEvent(ctx context.Context, r StartRequest, event Event) error {
	if event.Sequence < 1 || event.Sequence > 10000 {
		return ErrInvalid
	}
	allowed := map[string]bool{"running": true, "waiting": true, "needs_attention": true, "succeeded": true, "failed": true, "cancelled": true}
	if !allowed[event.State] {
		return ErrInvalid
	}
	if event.NodeID != "" && !validID(event.NodeID) {
		return ErrInvalid
	}
	if event.ErrorCode != "" && !validID(event.ErrorCode) {
		return ErrInvalid
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var checkpoint int64
	var state string
	err = tx.QueryRow(ctx, "SELECT checkpoint,state FROM runs WHERE workspace_id=$1 AND id=$2 AND execution_mode='execute' FOR UPDATE", r.WorkspaceID, r.RunID).Scan(&checkpoint, &state)
	if err != nil {
		return notFound(err)
	}
	if event.Sequence <= checkpoint {
		var equal bool
		err = tx.QueryRow(ctx, "SELECT event=$4::jsonb FROM run_events WHERE workspace_id=$1 AND run_id=$2 AND sequence=$3", r.WorkspaceID, r.RunID, event.Sequence, data).Scan(&equal)
		if err != nil {
			return err
		}
		if !equal {
			return ErrConflict
		}
		return tx.Commit(ctx)
	}
	if event.Sequence != checkpoint+1 || state == "succeeded" || state == "failed" || state == "cancelled" {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, "INSERT INTO run_events(workspace_id,run_id,sequence,event) VALUES($1,$2,$3,$4)", r.WorkspaceID, r.RunID, event.Sequence, data); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE runs SET state=$3,checkpoint=$4,updated_at=now() WHERE workspace_id=$1 AND id=$2", r.WorkspaceID, r.RunID, event.State, event.Sequence); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Events(ctx context.Context, workspace, run string, after int64) ([]RecordedEvent, error) {
	if after < 0 {
		return nil, ErrInvalid
	}
	if _, err := s.GetRun(ctx, workspace, run); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, "SELECT event,recorded_at FROM run_events WHERE workspace_id=$1 AND run_id=$2 AND sequence>$3 ORDER BY sequence LIMIT 200", workspace, run, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RecordedEvent{}
	for rows.Next() {
		var e RecordedEvent
		var raw []byte
		if err = rows.Scan(&raw, &e.RecordedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &e.Event); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
func (s *Store) RequestCancel(ctx context.Context, workspace, run string) (Run, error) {
	tag, err := s.Pool.Exec(ctx, "UPDATE runs SET cancel_requested=true WHERE workspace_id=$1 AND id=$2 AND execution_mode='execute' AND state NOT IN ('succeeded','failed','cancelled')", workspace, run)
	if err != nil {
		return Run{}, err
	}
	if tag.RowsAffected() == 0 {
		r, e := s.GetRun(ctx, workspace, run)
		if e != nil {
			return r, e
		}
		if r.ExecutionMode != "execute" {
			return r, ErrConflict
		}
		return r, nil
	}
	return s.GetRun(ctx, workspace, run)
}
func (s *Store) Attempts(ctx context.Context, workspace, run string) ([]Attempt, error) {
	if _, err := s.GetRun(ctx, workspace, run); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, "SELECT invocation_id,attempt,status,output_ref,error_code,completed_at FROM attempts WHERE workspace_id=$1 AND run_id=$2 ORDER BY invocation_id,attempt LIMIT 1000", workspace, run)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Attempt{}
	for rows.Next() {
		var a Attempt
		if err = rows.Scan(&a.InvocationID, &a.Attempt, &a.Status, &a.OutputRef, &a.ErrorCode, &a.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func mapScope(scope string) string {
	if scope == "root" {
		return ""
	}
	return "_" + scope
}
