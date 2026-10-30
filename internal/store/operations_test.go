package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
)

func TestReadinessAndScopedMetrics(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	if err := s.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	r, _ := executable(t, s)
	if err := s.SetRetentionHold(ctx, r.WorkspaceID, r.RunID, true); err != nil {
		t.Fatal(err)
	}
	m, err := s.OperationalMetrics(ctx, r.WorkspaceID)
	if err != nil || m.PendingStarts != 1 || m.RetentionHolds != 1 || m.Runs["accepted"] != 1 || m.PayloadBytes <= 0 {
		t.Fatal(m, err)
	}
	other, err := s.OperationalMetrics(ctx, workspace(t, s))
	if err != nil || other.PendingStarts != 0 || other.PayloadBytes != 0 {
		t.Fatal(other, err)
	}
	raw, _ := json.Marshal(m)
	if strings.Contains(string(raw), r.RunID) || strings.Contains(string(raw), r.WorkspaceID) {
		t.Fatal("metric identity leaked")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.OperationalMetrics(cancelled, r.WorkspaceID); err == nil {
		t.Fatal("ignored query cancellation")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Avoid exposing an inconsistent global migration table to unrelated tests.
	if _, err = tx.Exec(ctx, "UPDATE schema_migrations SET checksum='drift' WHERE version='013_retention.sql'"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Restore using the canonical embedded checksum even if the assertion fails.
	rawMigration, _ := migrations.ReadFile("migrations/013_retention.sql")
	expected := TokenHash(string(rawMigration))
	defer func() {
		if _, err := s.Pool.Exec(ctx, "UPDATE schema_migrations SET checksum=$1 WHERE version='013_retention.sql'", expected); err != nil {
			t.Error(err)
		}
	}()
	if err = s.CheckSchema(ctx); err == nil {
		t.Fatal("readiness accepted schema drift")
	}
}

type queryCounter struct{ queries int }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.queries++
	return ctx
}
func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestRunListUsesOneScopedQuery(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	for i := range 52 {
		if _, err := s.AdmitExecution(ctx, ws, w.ID, fmt.Sprintf("list_%d", i), []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE runs SET state='succeeded',history_expired_at=now() WHERE id=(SELECT min(id) FROM runs WHERE workspace_id=$1)", ws); err != nil {
		t.Fatal(err)
	}
	cfg := s.Pool.Config()
	counter := &queryCounter{}
	cfg.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	measured := Store{Pool: pool, Catalog: s.Catalog}
	first, err := measured.Runs(ctx, ws, w.ID, "")
	if err != nil || len(first) != 50 {
		t.Fatal(len(first), err)
	}
	if counter.queries != 1 {
		t.Fatalf("run list issued %d queries; want one snapshot", counter.queries)
	}
	for i, r := range first {
		if r.WorkflowID != w.ID || r.HistoryExpiredAt != nil || r.ExecutionMode != "execute" || r.DispatchState != "pending" || (i > 0 && first[i-1].ID >= r.ID) {
			t.Fatal("wrong run projection", r)
		}
	}
	next, err := measured.Runs(ctx, ws, w.ID, first[49].ID)
	if err != nil || len(next) != 1 || next[0].ID <= first[49].ID {
		t.Fatal(next, err)
	}
	foreign, err := measured.Runs(ctx, workspace(t, s), w.ID, "")
	if err != nil || len(foreign) != 0 {
		t.Fatal("workspace leak", foreign, err)
	}
}
func TestPayloadByteMetricMatchesRetainedBodies(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	r, _ := terminalFixture(t, s, 8)
	// Deduplicated payload insertion must not inflate the maintained counter.
	if _, err := s.PutPayload(ctx, r.WorkspaceID, r.RunID, map[string]any{"priority": 5}); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		var actual int64
		if err := s.Pool.QueryRow(ctx, "SELECT coalesce(sum(octet_length(data::text)),0) FROM payloads WHERE workspace_id=$1", r.WorkspaceID).Scan(&actual); err != nil {
			t.Fatal(err)
		}
		m, err := s.OperationalMetrics(ctx, r.WorkspaceID)
		if err != nil || m.PayloadBytes != actual {
			t.Fatal(m.PayloadBytes, actual, err)
		}
	}
	check()
	if _, err := s.RetainTerminalData(ctx, r.WorkspaceID, RetentionPolicy{7, 30, 100}, true); err != nil {
		t.Fatal(err)
	}
	check()
	if _, err := s.Pool.Exec(ctx, "UPDATE runs SET updated_at=now()-interval '31 days' WHERE id=$1", r.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetainTerminalData(ctx, r.WorkspaceID, RetentionPolicy{7, 30, 100}, true); err != nil {
		t.Fatal(err)
	}
	check()
}
