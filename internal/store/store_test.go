package store

import (
	"context"
	"github.com/google/uuid"
	"os"
	"testing"
)

func database(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("RUNNELIX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL: python3 tests/integration/postgres.py")
	}
	s, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}
func workspace(t *testing.T, s *Store) string {
	t.Helper()
	id := "ws_" + uuid.NewString()
	if _, err := s.Pool.Exec(context.Background(), "INSERT INTO workspaces(id,name) VALUES($1,'Fixture')", id); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestMigrations(t *testing.T) {
	s := database(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.Pool.QueryRow(context.Background(), "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("migration count %d: %v", count, err)
	}
}
