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

func TestCredentialRotationAndRevocation(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	box, _ := secrets.New(strings.Repeat("ab", 32), "test-key")
	c, err := s.CreateCredential(ctx, ws, "Rotate fixture", "bearer", "old-fixture", "slack.com", box)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantCredential(ctx, ws, c.ID, w.ID); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "bad\r\nvalue", strings.Repeat("x", 4097)} {
		if err = s.RotateCredential(ctx, ws, c.ID, value, box); err != ErrInvalid {
			t.Fatal("invalid rotation accepted", err)
		}
	}
	if err = s.RotateCredential(ctx, workspace(t, s), c.ID, "new-fixture", box); err != ErrNotFound {
		t.Fatal("scope escape", err)
	}
	if err = s.RotateCredential(ctx, ws, c.ID, "new-fixture", box); err != nil {
		t.Fatal(err)
	}
	value, err := s.ResolveCredential(ctx, ws, w.ID, c.ID, "bearer", "slack.com", box)
	if err != nil || value != "new-fixture" {
		t.Fatal("rotation did not preserve grant")
	}
	if _, err = s.ResolveCredential(ctx, ws, w.ID, c.ID, "bearer", "api.github.com", box); err != ErrNotFound {
		t.Fatal("destination changed")
	}
	if err = s.RevokeCredential(ctx, workspace(t, s), c.ID); err != ErrNotFound {
		t.Fatal("scope escape", err)
	}
	for i := 0; i < 2; i++ {
		if err = s.RevokeCredential(ctx, ws, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.RotateCredential(ctx, ws, c.ID, "revive-fixture", box); err != ErrNotFound {
		t.Fatal("revoked credential revived", err)
	}
	if _, err = s.ResolveCredential(ctx, ws, w.ID, c.ID, "bearer", "slack.com", box); err != ErrNotFound {
		t.Fatal("revoked credential resolved")
	}
}

func TestCredentialPagesIncludeLaterActiveRecords(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws := workspace(t, s)
	_, err := s.Pool.Exec(ctx, `INSERT INTO credentials(workspace_id,id,name,kind,ciphertext,key_id,destination,revoked)
 SELECT $1,'credential_'||lpad(i::text,3,'0'),'Fixture','bearer',decode('00','hex'),'fixture','slack.com',i<=100 FROM generate_series(1,101) i`, ws)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Credentials(ctx, ws, "")
	if err != nil || len(first) != 100 {
		t.Fatal("first page", len(first), err)
	}
	second, err := s.Credentials(ctx, ws, first[99].ID)
	if err != nil || len(second) != 1 || second[0].Revoked || second[0].ID != "credential_101" {
		t.Fatal("active credential missing from second page", err)
	}
	empty, err := s.Credentials(ctx, workspace(t, s), first[99].ID)
	if err != nil || len(empty) != 0 {
		t.Fatal("scope escape", err)
	}
}
