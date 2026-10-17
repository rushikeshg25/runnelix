package store

import (
	"context"
	"errors"
	"testing"
)

func TestUncertainResolutionIsExplicitImmutableAndFenced(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	r, snap := executable(t, s)
	e, err := s.PrepareInvocation(ctx, r, snap.Plan.Steps[1], snap.InputRef)
	if err != nil {
		t.Fatal(err)
	}
	res := Resolution{WorkspaceID: r.WorkspaceID, RunID: r.RunID, InvocationID: e.InvocationID, Action: "confirm", Actor: "fixture-operator", Note: "Verified provider fixture record"}
	if _, err = s.ResolveUncertain(ctx, res, map[string]any{"verified": true}); !errors.Is(err, ErrNotFound) {
		t.Fatal("non-uncertain resolution accepted", err)
	}
	if err = s.MarkUncertain(ctx, e); err != nil {
		t.Fatal(err)
	}
	recorded, err := s.ResolveUncertain(ctx, res, map[string]any{"verified": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolveUncertain(ctx, res, map[string]any{"verified": false}); !errors.Is(err, ErrConflict) {
		t.Fatal("decision overwritten", err)
	}
	pending, err := s.PendingResolutions(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatal("durable delivery missing", err)
	}
	accepted, err := s.AcceptResolution(ctx, e)
	if err != nil || accepted.OutputRef != recorded.OutputRef {
		t.Fatal(err)
	}
	if err = s.RecordAttempt(ctx, e, 1, "succeeded", snap.InputRef, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcceptAttempt(ctx, e, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("late attempt replaced operator result", err)
	}
	pending, err = s.PendingResolutions(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatal("resolution remained pending", err)
	}
}
