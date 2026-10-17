package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunBudgetsAndSharedProviderLimit(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	r, snap := executable(t, s)
	if _, err := s.Pool.Exec(ctx, "UPDATE runs SET max_invocations=2,max_payload_bytes=8192 WHERE workspace_id=$1 AND id=$2", r.WorkspaceID, r.RunID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.PrepareScopedInvocation(ctx, r, snap.Plan.Steps[1], snap.InputRef, fmt.Sprintf("item_%d", i))
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 2 {
		t.Fatal("invocation cap", accepted.Load())
	}
	if _, err := s.PutPayload(ctx, r.WorkspaceID, r.RunID, map[string]any{"value": strings.Repeat("a", 4100)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutPayload(ctx, r.WorkspaceID, r.RunID, map[string]any{"value": strings.Repeat("b", 4100)}); !errors.Is(err, ErrLimit) {
		t.Fatal("payload quota bypass", err)
	}
	if _, err := s.PutPayload(ctx, r.WorkspaceID, r.RunID, map[string]any{"value": strings.Repeat("a", 4100)}); err != nil {
		t.Fatal("dedup counted twice", err)
	}
	accepted.Store(0)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.AcquireProvider(ctx, r.WorkspaceID, "fixture.example", "credential_fixture")
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("shared rate limit", accepted.Load())
	}
	time.Sleep(210 * time.Millisecond)
	if err := s.AcquireProvider(ctx, r.WorkspaceID, "fixture.example", "credential_fixture"); err != nil {
		t.Fatal("limit did not refill", err)
	}
}
