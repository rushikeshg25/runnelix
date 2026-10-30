package store

import (
	"context"
	"testing"
	"time"
)

func TestScheduleAdmissionNoOverlapRevisionAndDST(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	spec := ScheduleSpec{Kind: "daily", Hour: 1, Minute: 30, Timezone: "America/New_York", Enabled: true, Input: []byte(`{}`)}
	schedule, err := s.SaveSchedule(ctx, Schedule{WorkspaceID: ws, WorkflowID: w.ID, Spec: spec}, 0)
	if err != nil {
		t.Fatal(err)
	}
	firstUTC := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	repeatUTC := firstUTC.Add(time.Hour)
	first, err := s.AdmitSchedule(ctx, ws, schedule.ID, 1, firstUTC)
	if err != nil || first.Outcome != "accepted" {
		t.Fatal(first, err)
	}
	repeat, err := s.AdmitSchedule(ctx, ws, schedule.ID, 1, repeatUTC)
	if err != nil || repeat.RunID != first.RunID {
		t.Fatal("repeated DST hour duplicated", repeat, err)
	}
	next, err := s.AdmitSchedule(ctx, ws, schedule.ID, 1, firstUTC.Add(24*time.Hour))
	if err != nil || next.Outcome != "overlap" {
		t.Fatal("overlap", next, err)
	}
	// The active run is the durable slot. Only a confirmed terminal projection releases it.
	if err = s.ApplyEvent(ctx, StartRequest{WorkspaceID: ws, RunID: first.RunID}, Event{Sequence: 1, State: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	next, err = s.AdmitSchedule(ctx, ws, schedule.ID, 1, firstUTC.Add(48*time.Hour))
	if err != nil || next.Outcome != "accepted" {
		t.Fatal("slot not released", next, err)
	}
	spec.Enabled = false
	schedule.Spec = spec
	schedule, err = s.SaveSchedule(ctx, schedule, 1)
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := s.AdmitSchedule(ctx, ws, schedule.ID, 1, firstUTC.Add(72*time.Hour))
	if err != nil || disabled.Outcome != "disabled" {
		t.Fatal("disabled admitted", disabled, err)
	}
	if _, err = s.SaveSchedule(ctx, schedule, 1); err != ErrConflict {
		t.Fatal("stale update accepted", err)
	}
	admissions := 0
	for _, record := range auditRows(t, s, ws) {
		if record.Action == "run.admitted" && record.Details.Source == "schedule" {
			admissions++
			if record.Actor != "system" || record.RequestID != "" {
				t.Fatal("scheduled admission claimed a human identity")
			}
		}
	}
	if admissions != 2 {
		t.Fatal("scheduled admission audit missing or duplicated", admissions)
	}
}
func TestScheduleValidationAndSyncReplay(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	spec := ScheduleSpec{Kind: "interval", IntervalSeconds: 60, Timezone: "UTC", Enabled: true, Input: []byte(`{}`)}
	schedule, err := s.SaveSchedule(ctx, Schedule{WorkspaceID: ws, WorkflowID: w.ID, Spec: spec}, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	apply := func(item Schedule) error {
		if item.ID == schedule.ID {
			calls++
			if item.Revision != 1 {
				t.Fatal("wrong target revision")
			}
		}
		return nil
	}
	for range 30 {
		changed, err := s.SyncSchedule(ctx, apply)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			break
		}
	}
	if calls != 1 {
		t.Fatal("target desired revision was not applied exactly once", calls)
	}
	if changed, err := s.SyncSchedule(ctx, apply); err != nil || changed || calls != 1 {
		t.Fatal("observed schedule resent", err)
	}
	for _, bad := range []ScheduleSpec{{Kind: "interval", IntervalSeconds: 1, Timezone: "UTC", Input: []byte(`{}`)}, {Kind: "daily", Hour: 24, Timezone: "UTC", Input: []byte(`{}`)}, {Kind: "daily", Timezone: "unknown", Input: []byte(`{}`)}} {
		if validateSchedule(bad) == nil {
			t.Fatal("invalid schedule accepted")
		}
	}
}
