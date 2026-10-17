package store

import (
	"context"
	"github.com/rushikeshg25/runnelix/internal/secrets"
	"strings"
	"testing"
)

func TestCredentialEncryptionGrantsAndDestination(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	box, err := secrets.New(strings.Repeat("ab", 32), "test-key")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateCredential(ctx, ws, "Test bearer", "bearer", "sentinel-test-token", "example.com", box)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolveCredential(ctx, ws, w.ID, c.ID, "bearer", "example.com", box); err == nil {
		t.Fatal("ungranted credential resolved")
	}
	if err = s.GrantCredential(ctx, ws, c.ID, w.ID); err != nil {
		t.Fatal(err)
	}
	value, err := s.ResolveCredential(ctx, ws, w.ID, c.ID, "bearer", "example.com", box)
	if err != nil || value != "sentinel-test-token" {
		t.Fatal("granted credential unavailable")
	}
	for _, destination := range []string{"other.example", ""} {
		if _, err = s.ResolveCredential(ctx, ws, w.ID, c.ID, "bearer", destination, box); err == nil {
			t.Fatal("destination escape")
		}
	}
	var data []byte
	if err = s.Pool.QueryRow(ctx, "SELECT ciphertext FROM credentials WHERE workspace_id=$1 AND id=$2", ws, c.ID).Scan(&data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), value) {
		t.Fatal("plaintext persisted")
	}
	if _, err = s.ResolveCredential(ctx, "other", w.ID, c.ID, "bearer", "example.com", box); err == nil {
		t.Fatal("scope escape")
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE credentials SET revoked=true WHERE workspace_id=$1 AND id=$2", ws, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolveCredential(ctx, ws, w.ID, c.ID, "bearer", "example.com", box); err == nil {
		t.Fatal("revoked credential resolved")
	}
}
