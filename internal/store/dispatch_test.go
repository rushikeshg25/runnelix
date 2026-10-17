package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLeasesReceiptsAndFencing(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	run, err := s.Admit(ctx, ws, w.ID, "event_1", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	// Other tests leave pending rows; isolate this candidate by leasing all pending rows first.
	if _, err = s.Pool.Exec(ctx, "UPDATE start_outbox SET next_try=now()+interval '1 hour' WHERE run_id<>$1", run.ID); err != nil {
		t.Fatal(err)
	}
	first, err := s.Claim(ctx, time.Second)
	if err != nil || first.RunID != run.ID {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx, time.Second); !errors.Is(err, ErrNotFound) {
		t.Fatal("lease claimed twice", err)
	}
	if err = s.Acknowledge(ctx, first); !errors.Is(err, ErrConflict) {
		t.Fatal("ack without receipt", err)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE start_outbox SET lease_until=now()-interval '1 second' WHERE run_id=$1", run.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.Claim(ctx, 10*time.Second)
	if err != nil || second.Token == first.Token {
		t.Fatal("lease not replaced", err)
	}
	forged := second.StartRequest
	forged.PlanDigest = "bad"
	if err = s.RecordReceipt(ctx, forged, RuntimeID(run.ID)); !errors.Is(err, ErrConflict) {
		t.Fatal("forged digest accepted", err)
	}
	if err = s.RecordReceipt(ctx, second.StartRequest, RuntimeID(run.ID)); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordReceipt(ctx, second.StartRequest, RuntimeID(run.ID)); err != nil {
		t.Fatal("receipt not idempotent", err)
	}
	if err = s.Acknowledge(ctx, first); !errors.Is(err, ErrConflict) {
		t.Fatal("stale ack accepted", err)
	}
	if err = s.Retry(ctx, first, time.Second); !errors.Is(err, ErrConflict) {
		t.Fatal("stale retry accepted", err)
	}
	if err = s.Acknowledge(ctx, second); err != nil {
		t.Fatal(err)
	}
	state, err := s.GetRun(ctx, ws, run.ID)
	if err != nil || state.DispatchState != "acknowledged" || state.State != "accepted" {
		t.Fatal("receipt confused with execution", state, err)
	}
}
