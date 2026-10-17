package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func executable(t *testing.T, s *Store) (StartRequest, Snapshot) {
	t.Helper()
	ctx := context.Background()
	ws, w, _ := published(t, s)
	run, err := s.AdmitExecution(ctx, ws, w.ID, "event_execution", []byte(`{"priority":5}`))
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.GetVersion(ctx, ws, w.ID, run.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	r := StartRequest{WorkspaceID: ws, RunID: run.ID, VersionID: v.ID, PlanDigest: v.Digest, ExecutionMode: "execute"}
	snapshot, err := s.ExecutionSnapshot(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	return r, snapshot
}
func TestExecutionModePreservesAdmission(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	first, err := s.Admit(ctx, ws, w.ID, "same", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdmitExecution(ctx, ws, w.ID, "same", []byte(`{}`)); !errors.Is(err, ErrConflict) {
		t.Fatal("mode silently changed", err)
	}
	r, snap := executable(t, s)
	if snap.InputRef == "" || RuntimeIDFor(r) == RuntimeID(r.RunID) || first.ExecutionMode != "admission" {
		t.Fatal("mode identity failed")
	}
	if err = s.RecordReceipt(ctx, r, RuntimeIDFor(r)); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordReceipt(ctx, r, RuntimeID(r.RunID)); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong runtime accepted")
	}
}
func TestImmutableAttemptSelectionAndScope(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	r, snap := executable(t, s)
	step := snap.Plan.Steps[1]
	e, err := s.PrepareInvocation(ctx, r, step, snap.InputRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Payload(ctx, "other", r.RunID, snap.InputRef); !errors.Is(err, ErrNotFound) {
		t.Fatal("payload scope leak")
	}
	forged := e
	forged.InputRef = "payload_forged"
	if err = s.Invocation(ctx, forged); !errors.Is(err, ErrConflict) {
		t.Fatal("forged task accepted")
	}
	ref1, err := s.PutPayload(ctx, r.WorkspaceID, r.RunID, map[string]any{"result": 1})
	if err != nil {
		t.Fatal(err)
	}
	ref2, err := s.PutPayload(ctx, r.WorkspaceID, r.RunID, map[string]any{"result": 2})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordAttempt(ctx, e, 2, "succeeded", ref2, ""); err != nil {
		t.Fatal(err)
	}
	if ref, err := s.AcceptAttempt(ctx, e, 2); err != nil || ref != ref2 {
		t.Fatal("winner selection", err)
	}
	if err = s.RecordAttempt(ctx, e, 1, "succeeded", ref1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcceptAttempt(ctx, e, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("late worker changed winner")
	}
	if err = s.RecordAttempt(ctx, e, 2, "succeeded", ref1, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("attempt overwritten")
	}
	if err = s.RecordAttempt(ctx, e, 2, "succeeded", ref2, ""); err != nil {
		t.Fatal("attempt replay", err)
	}
}
func TestEventsMonotonicAndCancellation(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	r, _ := executable(t, s)
	first := Event{Sequence: 1, State: "running"}
	if err := s.ApplyEvent(ctx, r, first); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyEvent(ctx, r, first); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyEvent(ctx, r, Event{Sequence: 3, State: "running"}); !errors.Is(err, ErrConflict) {
		t.Fatal("gap accepted")
	}
	if _, err := s.RequestCancel(ctx, r.WorkspaceID, r.RunID); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyEvent(ctx, r, Event{Sequence: 2, State: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyEvent(ctx, r, Event{Sequence: 3, State: "running"}); !errors.Is(err, ErrConflict) {
		t.Fatal("terminal run revived")
	}
	events, err := s.Events(ctx, r.WorkspaceID, r.RunID, 0)
	if err != nil || len(events) != 2 || events[1].RecordedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("events", err)
	}
	raw, _ := json.Marshal(events)
	if len(raw) == 0 {
		t.Fatal("empty events")
	}
}

func TestWebhookSourceIsolation(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	manual, err := s.AdmitExecution(ctx, ws, w.ID, "event_one", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	hook, err := s.AdmitWebhook(ctx, ws, w.ID, "event_one", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if manual.ID == hook.ID {
		t.Fatal("trigger sources collided")
	}
	duplicate, err := s.AdmitWebhook(ctx, ws, w.ID, "event_one", []byte(`{}`))
	if err != nil || duplicate.ID != hook.ID {
		t.Fatal("webhook not deduplicated", err)
	}
	if _, err = s.AdmitWebhook(ctx, ws, w.ID, "event_one", []byte(`{"changed":true}`)); !errors.Is(err, ErrConflict) {
		t.Fatal("changed signed delivery accepted")
	}
}
