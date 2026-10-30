package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rushikeshg25/runnelix/internal/identity"
)

func TestLoginChallengeConsumedOnce(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	hash := TokenHash(uuid.NewString())
	c := LoginChallenge{StateHash: hash, Verifier: "fixture-verifier", Nonce: "fixture-nonce", Issuer: "https://fixture.example", ClientID: "fixture"}
	if err := s.CreateLogin(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeLogin(ctx, hash, c.Issuer, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatal("client mismatch accepted", err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.ConsumeLogin(ctx, hash, c.Issuer, c.ClientID); results <- err }()
	}
	success, missing := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, ErrNotFound) {
			missing++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || missing != 1 {
		t.Fatalf("success=%d missing=%d", success, missing)
	}
}
func TestSessionBindingRotationAndExpiry(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	p := identity.Principal{Issuer: "https://fixture.example", Subject: "fixture"}
	first, second := TokenHash(uuid.NewString()), TokenHash(uuid.NewString())
	if err := s.CreateSession(ctx, first, "", p, "client", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"https://other.example", "client"}, {p.Issuer, "other-client"}} {
		if _, _, err := s.Session(ctx, first, pair[0], pair[1]); !errors.Is(err, ErrNotFound) {
			t.Fatal("session binding bypass", err)
		}
	}
	if err := s.CreateSession(ctx, second, first, p, "client", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Session(ctx, first, p.Issuer, "client"); !errors.Is(err, ErrNotFound) {
		t.Fatal("old session survived rotation", err)
	}
	if got, _, err := s.Session(ctx, second, p.Issuer, "client"); err != nil || got != p {
		t.Fatal("new session missing", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE browser_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, second); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Session(ctx, second, p.Issuer, "client"); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired session accepted", err)
	}
}
