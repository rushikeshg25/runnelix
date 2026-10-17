package store

import (
	"context"
	"errors"
	"time"
)

var ErrLimit = errors.New("execution limit exceeded")

// Provider and credential buckets are shared by every service replica in this workspace.
func (s *Store) AcquireProvider(ctx context.Context, workspace, host, credential string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	buckets := []struct {
		key   string
		delay time.Duration
	}{{"host/" + host, 100 * time.Millisecond}}
	if credential != "" {
		buckets = append(buckets, struct {
			key   string
			delay time.Duration
		}{"credential/" + credential, 200 * time.Millisecond})
	}
	for _, b := range buckets {
		tag, err := tx.Exec(ctx, `INSERT INTO provider_limits(workspace_id,bucket,next_allowed_at) VALUES($1,$2,now()+($3*interval '1 millisecond')) ON CONFLICT(workspace_id,bucket) DO UPDATE SET next_allowed_at=EXCLUDED.next_allowed_at WHERE provider_limits.next_allowed_at<=now()`, workspace, b.key, b.delay.Milliseconds())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrLimit
		}
	}
	return tx.Commit(ctx)
}
