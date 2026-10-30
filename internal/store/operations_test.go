package store

import (
	"context"
	"encoding/json"
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
