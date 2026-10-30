package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/rushikeshg25/runnelix/internal/identity"
)

func TokenHash(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

type LoginChallenge struct{ StateHash, Verifier, Nonce, Issuer, ClientID string }

func (s *Store) CreateLogin(ctx context.Context, c LoginChallenge) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(78134013)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM login_challenges WHERE expires_at<=now()`); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM login_challenges`).Scan(&count); err != nil {
		return err
	}
	if count >= 1000 {
		return ErrLimit
	}
	_, err = tx.Exec(ctx, `INSERT INTO login_challenges(state_hash,verifier,nonce,issuer,client_id,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '10 minutes')`, c.StateHash, c.Verifier, c.Nonce, c.Issuer, c.ClientID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) ConsumeLogin(ctx context.Context, hash, issuer, client string) (LoginChallenge, error) {
	var c LoginChallenge
	err := s.Pool.QueryRow(ctx, `DELETE FROM login_challenges WHERE state_hash=$1 AND issuer=$2 AND client_id=$3 AND expires_at>now() RETURNING verifier,nonce`, hash, issuer, client).Scan(&c.Verifier, &c.Nonce)
	return c, notFound(err)
}
func (s *Store) CreateSession(ctx context.Context, hash, previous string, p identity.Principal, client string, expires time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(78134014)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM browser_sessions WHERE expires_at<=now() OR token_hash=$1`, previous); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM browser_sessions`).Scan(&count); err != nil {
		return err
	}
	if count >= 10000 {
		return ErrLimit
	}
	_, err = tx.Exec(ctx, `INSERT INTO browser_sessions(token_hash,issuer,subject,client_id,expires_at) VALUES($1,$2,$3,$4,$5)`, hash, p.Issuer, p.Subject, client, expires)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Session(ctx context.Context, hash, issuer, client string) (identity.Principal, time.Time, error) {
	var p identity.Principal
	var expires time.Time
	err := s.Pool.QueryRow(ctx, `SELECT issuer,subject,expires_at FROM browser_sessions WHERE token_hash=$1 AND issuer=$2 AND client_id=$3 AND expires_at>now()`, hash, issuer, client).Scan(&p.Issuer, &p.Subject, &expires)
	return p, expires, notFound(err)
}
func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM browser_sessions WHERE token_hash=$1`, hash)
	return err
}
