package store

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
	"time"
)

func terminalFixture(t *testing.T, s *Store, days int) (StartRequest, Snapshot) {
	t.Helper()
	r, snap := executable(t, s)
	ctx := context.Background()
	e, err := s.PrepareInvocation(ctx, r, snap.Plan.Steps[1], snap.InputRef)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordAttempt(ctx, e, 1, "succeeded", snap.InputRef, ""); err != nil {
		t.Fatal(err)
	}
	if err = s.MarkUncertain(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolveUncertain(ctx, Resolution{WorkspaceID: r.WorkspaceID, RunID: r.RunID, InvocationID: e.InvocationID, Action: "confirm", Actor: "fixture", Note: "retained note"}, map[string]any{"priority": 5}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcceptResolution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyEvent(ctx, r, Event{Sequence: 1, State: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordReceipt(ctx, r, RuntimeIDFor(r)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE start_outbox SET dispatched_at=now() WHERE run_id=$1", r.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE runs SET updated_at=now()-($2*interval '1 day') WHERE id=$1", r.RunID, days); err != nil {
		t.Fatal(err)
	}
	return r, snap
}
func TestRetentionExpiryAndReplay(t *testing.T) {
	s := database(t)
	ctx := WithAuditContext(context.Background(), "installation-operator", "")
	r, snap := terminalFixture(t, s, 8)
	policy := RetentionPolicy{7, 30, 100}
	preview, err := s.RetainTerminalData(ctx, r.WorkspaceID, policy, false)
	if err != nil || preview.Applied || len(preview.Candidates) != 1 || preview.Candidates[0].ExpireHistory {
		t.Fatal(preview, err)
	}
	if _, err = s.Payload(ctx, r.WorkspaceID, r.RunID, snap.InputRef); err != nil {
		t.Fatal("preview mutated data", err)
	}
	before := len(auditRows(t, s, r.WorkspaceID))
	result, err := s.RetainTerminalData(ctx, r.WorkspaceID, policy, true)
	if err != nil || !result.Applied || len(result.Candidates) != 1 {
		t.Fatal(result, err)
	}
	if _, err = s.Payload(ctx, r.WorkspaceID, r.RunID, snap.InputRef); !errors.Is(err, ErrExpired) {
		t.Fatal("payload not expired", err)
	}
	if _, err = s.Payload(ctx, "other", r.RunID, snap.InputRef); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-workspace leak", err)
	}
	if _, err = s.PutPayload(ctx, r.WorkspaceID, r.RunID, map[string]any{"priority": 5}); !errors.Is(err, ErrExpired) {
		t.Fatal("stale writer restored payload", err)
	}
	run, err := s.GetRun(ctx, r.WorkspaceID, r.RunID)
	if err != nil || run.PayloadsExpiredAt == nil || run.HistoryExpiredAt != nil {
		t.Fatal(run, err)
	}
	var input string
	if err = s.Pool.QueryRow(ctx, "SELECT input::text FROM runs WHERE id=$1", r.RunID).Scan(&input); err != nil || input != "null" {
		t.Fatal("trigger input retained", input, err)
	}
	if result, err = s.RetainTerminalData(ctx, r.WorkspaceID, policy, true); err != nil || len(result.Candidates) != 0 {
		t.Fatal("expiry duplicated", result, err)
	}
	if len(auditRows(t, s, r.WorkspaceID)) != before+1 {
		t.Fatal("wrong audit count")
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE runs SET updated_at=now()-interval '31 days' WHERE id=$1", r.RunID); err != nil {
		t.Fatal(err)
	}
	if result, err = s.RetainTerminalData(ctx, r.WorkspaceID, policy, true); err != nil || len(result.Candidates) != 1 || !result.Candidates[0].ExpireHistory {
		t.Fatal(result, err)
	}
	if _, err = s.GetRun(ctx, r.WorkspaceID, r.RunID); !errors.Is(err, ErrExpired) {
		t.Fatal("history still visible", err)
	}
	if runs, err := s.Runs(ctx, r.WorkspaceID, run.WorkflowID, ""); err != nil || len(runs) != 0 {
		t.Fatal(runs, err)
	}
	for _, table := range []string{"payloads", "node_invocations", "attempts", "run_events", "uncertain_invocations", "operator_resolutions"} {
		var count int
		if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE workspace_id=$1 AND run_id=$2", r.WorkspaceID, r.RunID).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	replay, err := s.AdmitExecution(ctx, r.WorkspaceID, run.WorkflowID, "event_execution", []byte(`{"priority":5}`))
	if err != nil || replay.ID != r.RunID || replay.HistoryExpiredAt == nil {
		t.Fatal("lost dedup receipt", replay, err)
	}
	if _, err = s.AdmitExecution(ctx, r.WorkspaceID, run.WorkflowID, "event_execution", []byte(`{"priority":6}`)); !errors.Is(err, ErrConflict) {
		t.Fatal("lost fingerprint", err)
	}
	if present, err := s.HasReceipt(ctx, r); err != nil || !present {
		t.Fatal("runtime receipt removed", err)
	}
	if len(auditRows(t, s, r.WorkspaceID)) != before+2 {
		t.Fatal("audit history lost")
	}
}
func TestRetentionProtectionAndRollback(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	policy := RetentionPolicy{7, 30, 100}
	for _, state := range []string{"accepted", "running", "waiting", "needs_attention"} {
		r, _ := executable(t, s)
		if _, err := s.Pool.Exec(ctx, "UPDATE runs SET state=$2,updated_at=now()-interval '60 days' WHERE id=$1", r.RunID, state); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Pool.Exec(ctx, "UPDATE start_outbox SET dispatched_at=now() WHERE run_id=$1", r.RunID); err != nil {
			t.Fatal(err)
		}
		result, err := s.RetainTerminalData(ctx, r.WorkspaceID, policy, true)
		if err != nil || len(result.Candidates) != 0 {
			t.Fatal("active run selected", state, result, err)
		}
	}
	r, snap := terminalFixture(t, s, 31)
	if err := s.SetRetentionHold(ctx, r.WorkspaceID, r.RunID, true); err != nil {
		t.Fatal(err)
	}
	if result, err := s.RetainTerminalData(ctx, r.WorkspaceID, policy, true); err != nil || len(result.Candidates) != 0 {
		t.Fatal("hold ignored", result, err)
	}
	if err := s.SetRetentionHold(ctx, r.WorkspaceID, r.RunID, false); err != nil {
		t.Fatal(err)
	}
	rejectAudit(t, s, r.WorkspaceID)
	if _, err := s.RetainTerminalData(ctx, r.WorkspaceID, policy, true); err == nil {
		t.Fatal("audit failure committed expiry")
	}
	if _, err := s.Payload(ctx, r.WorkspaceID, r.RunID, snap.InputRef); err != nil {
		t.Fatal("rollback lost payload", err)
	}
	if run, err := s.GetRun(ctx, r.WorkspaceID, r.RunID); err != nil || run.PayloadsExpiredAt != nil || run.HistoryExpiredAt != nil {
		t.Fatal("rollback lost history", run, err)
	}
}
func TestRetentionPinsUncertainAndPendingDispatch(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	policy := RetentionPolicy{7, 30, 100}
	r, snap := executable(t, s)
	e, err := s.PrepareInvocation(ctx, r, snap.Plan.Steps[1], snap.InputRef)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkUncertain(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err = s.ApplyEvent(ctx, r, Event{Sequence: 1, State: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE runs SET updated_at=now()-interval '60 days' WHERE id=$1", r.RunID); err != nil {
		t.Fatal(err)
	}
	if result, err := s.RetainTerminalData(ctx, r.WorkspaceID, policy, true); err != nil || len(result.Candidates) != 0 {
		t.Fatal("pending dispatch selected", result, err)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE start_outbox SET dispatched_at=now() WHERE run_id=$1", r.RunID); err != nil {
		t.Fatal(err)
	}
	if result, err := s.RetainTerminalData(ctx, r.WorkspaceID, policy, true); err != nil || len(result.Candidates) != 0 {
		t.Fatal("unresolved decision selected", result, err)
	}
}
func TestRetentionLocksAndBounds(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	r, _ := terminalFixture(t, s, 31)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT id FROM runs WHERE id=$1 FOR UPDATE", r.RunID); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if result, err := s.RetainTerminalData(bounded, r.WorkspaceID, RetentionPolicy{7, 30, 1}, true); err != nil || len(result.Candidates) != 0 {
		t.Fatal("locked row not skipped", result, err)
	}
	for _, p := range []RetentionPolicy{{0, 30, 1}, {7, 6, 1}, {7, 30, 0}, {7, 30, 101}, {7, 36501, 1}} {
		if _, err = s.RetainTerminalData(ctx, r.WorkspaceID, p, true); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid policy accepted", p, err)
		}
	}
}

func TestRetentionUsesElapsedDaysAcrossDST(t *testing.T) {
	for _, tc := range []struct {
		name, now string
		age       time.Duration
		want      int
	}{
		{"spring", "2026-03-10T12:00:00-04:00", 7*24*time.Hour - 30*time.Minute, 0},
		{"fall", "2026-11-03T12:00:00-05:00", 7*24*time.Hour + 30*time.Minute, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := database(t)
			ctx := context.Background()
			r, _ := terminalFixture(t, s, 8)
			fixed, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Pool.Exec(ctx, "UPDATE runs SET updated_at=$2 WHERE id=$1", r.RunID, fixed.Add(-tc.age)); err != nil {
				t.Fatal(err)
			}
			schema := "retention_clock_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if _, err = s.Pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
				t.Fatal(err)
			}
			defer s.Pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
			if _, err = s.Pool.Exec(ctx, "CREATE FUNCTION "+schema+".now() RETURNS timestamptz LANGUAGE sql STABLE AS $$ SELECT '"+tc.now+"'::timestamptz $$"); err != nil {
				t.Fatal(err)
			}
			cfg := s.Pool.Config()
			cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",pg_catalog,public"
			cfg.ConnConfig.RuntimeParams["TimeZone"] = "America/New_York"
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			clockStore := Store{Pool: pool, Catalog: s.Catalog}
			result, err := clockStore.RetainTerminalData(ctx, r.WorkspaceID, RetentionPolicy{7, 30, 100}, false)
			if err != nil || len(result.Candidates) != tc.want {
				t.Fatalf("elapsed age %s: got %d candidates, want %d: %v", tc.age, len(result.Candidates), tc.want, err)
			}
		})
	}
}
