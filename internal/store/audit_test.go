package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/identity"
	"github.com/rushikeshg25/runnelix/internal/secrets"
)

func auditRows(t *testing.T, s *Store, ws string) []AuditRecord {
	t.Helper()
	rows, err := s.AuditRecords(context.Background(), ws, 0)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
func TestAuditActionsExcludeSensitiveValuesAndDeduplicateRetries(t *testing.T) {
	s := database(t)
	ws := workspace(t, s)
	owner := identity.Principal{Issuer: "https://audit-fixture.example", Subject: "sensitive-subject-sentinel"}
	ctx := WithAuditContext(context.Background(), PrincipalActor(owner), "request_fixture")
	if err := s.ProvisionOwner(WithAuditContext(ctx, "installation-operator", ""), ws, owner); err != nil {
		t.Fatal(err)
	}
	member := Membership{Issuer: owner.Issuer, Subject: "private-member-sentinel", Role: "operator", DataRead: true}
	if err := s.ChangeMembership(ctx, ws, owner, member, false); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeMembership(ctx, ws, owner, member, true); err != nil {
		t.Fatal(err)
	}
	w, err := s.CreateWorkflow(ctx, ws, "private-workflow-name-sentinel", graph)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveDraft(ctx, ws, w.ID, 1, graph); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err = s.Publish(ctx, ws, w.ID, 2); err != nil {
			t.Fatal(err)
		}
	}
	box, _ := secrets.New(strings.Repeat("ab", 32), "fixture-key")
	c, err := s.CreateCredential(ctx, ws, "private-credential-name-sentinel", "bearer", "private-token-sentinel", "private-host.example", box)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = s.GrantCredential(ctx, ws, c.ID, w.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.RotateCredential(ctx, ws, c.ID, "private-rotated-token-sentinel", box); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = s.RevokeCredential(ctx, ws, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	var run Run
	for range 2 {
		run, err = s.AdmitExecution(ctx, ws, w.ID, "private-idempotency-sentinel", []byte(`{"input":"private-input-sentinel"}`))
		if err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err = s.RequestCancel(ctx, ws, run.ID); err != nil {
			t.Fatal(err)
		}
	}
	spec := ScheduleSpec{Kind: "interval", IntervalSeconds: 60, Timezone: "UTC", Enabled: true, Input: []byte(`{"private":"private-schedule-sentinel"}`)}
	if _, err = s.SaveSchedule(ctx, Schedule{WorkspaceID: ws, WorkflowID: w.ID, Spec: spec}, 0); err != nil {
		t.Fatal(err)
	}
	bundle, err := definition.ExportPortable("private-import-name-sentinel", graph, s.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportWorkflow(ctx, ws, bundle, nil); err != nil {
		t.Fatal(err)
	}
	rows := auditRows(t, s, ws)
	counts := map[string]int{}
	for i, row := range rows {
		if row.Sequence != int64(i+1) || row.RecordedAt.IsZero() {
			t.Fatal("invalid sequence/time")
		}
		counts[row.Action]++
		if i > 0 && (row.Actor != PrincipalActor(owner) || row.RequestID != "request_fixture") {
			t.Fatal("lost actor/request attribution")
		}
	}
	for _, action := range []string{"membership.provisioned", "membership.set", "membership.removed", "workflow.created", "workflow.draft_saved", "workflow.published", "credential.created", "credential.granted", "credential.rotated", "credential.revoked", "run.admitted", "run.cancel_requested", "schedule.saved", "workflow.imported"} {
		if counts[action] != 1 {
			t.Fatalf("%s count %d", action, counts[action])
		}
	}
	raw, _ := json.Marshal(rows)
	for _, sentinel := range []string{"sensitive-subject-sentinel", "private-", "private-host.example", "fixture-key"} {
		if strings.Contains(string(raw), sentinel) {
			t.Fatal("sensitive data reached audit")
		}
	}
	if other := auditRows(t, s, workspace(t, s)); len(other) != 0 {
		t.Fatal("cross-workspace audit leak")
	}
}

func TestAuditReconciliationExcludesNoteAndOutput(t *testing.T) {
	s := database(t)
	r, snap := executable(t, s)
	ctx := WithAuditContext(context.Background(), "test-operator", "request_resolution")
	e, err := s.PrepareInvocation(ctx, r, snap.Plan.Steps[1], snap.InputRef)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkUncertain(ctx, e); err != nil {
		t.Fatal(err)
	}
	resolution := Resolution{WorkspaceID: r.WorkspaceID, RunID: r.RunID, InvocationID: e.InvocationID, Action: "confirm", Actor: "test-operator", Note: "private-note-sentinel"}
	for range 2 {
		if _, err = s.ResolveUncertain(ctx, resolution, map[string]any{"secret": "private-output-sentinel"}); err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for _, row := range auditRows(t, s, r.WorkspaceID) {
		if row.Action == "run.resolved" {
			count++
			if row.Actor != "test-operator" || row.Details.RunID != r.RunID || row.Details.Decision != "confirm" || row.TargetID != e.InvocationID {
				t.Fatal("resolution metadata missing")
			}
			raw, _ := json.Marshal(row)
			if strings.Contains(string(raw), "private-") {
				t.Fatal("resolution data leaked")
			}
		}
	}
	if count != 1 {
		t.Fatal("resolution replay duplicated audit")
	}
	// Complete the fixture's durable delivery so global pending-resolution
	// checks in the existing suite do not inherit this test's work.
	if _, err = s.AcceptResolution(ctx, e); err != nil {
		t.Fatal(err)
	}
}

func TestAuditAppendOnlyAndRollback(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws := workspace(t, s)
	w, err := s.CreateWorkflow(ctx, ws, "Fixture", graph)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`UPDATE audit_records SET actor='changed' WHERE workspace_id=$1`, `DELETE FROM audit_records WHERE workspace_id=$1`, `TRUNCATE audit_records`} {
		var err error
		if strings.Contains(query, "$1") {
			_, err = s.Pool.Exec(ctx, query, ws)
		} else {
			_, err = s.Pool.Exec(ctx, query)
		}
		if err == nil {
			t.Fatal("audit mutation accepted")
		}
	}
	rejectAudit(t, s, ws)
	if _, err = s.SaveDraft(ctx, ws, w.ID, 1, graph); err == nil {
		t.Fatal("write survived audit failure")
	}
	got, err := s.GetWorkflow(ctx, ws, w.ID)
	if err != nil || got.Revision != 1 {
		t.Fatal("draft mutation was not rolled back", err)
	}
	if _, err = s.Publish(ctx, ws, w.ID, 1); err == nil {
		t.Fatal("publication survived audit failure")
	}
	got, err = s.GetWorkflow(ctx, ws, w.ID)
	if err != nil || got.ActiveVersionID != nil {
		t.Fatal("publication was not rolled back", err)
	}
	box, _ := secrets.New(strings.Repeat("ab", 32), "fixture")
	if _, err = s.CreateCredential(ctx, ws, "Fixture", "bearer", "fixture-token", "example.com", box); err == nil {
		t.Fatal("credential survived audit failure")
	}
	credentials, err := s.Credentials(ctx, ws, "")
	if err != nil || len(credentials) != 0 {
		t.Fatal("credential mutation was not rolled back", err)
	}
	owner := identity.Principal{Issuer: "https://fixture.example", Subject: "owner"}
	if err = s.ProvisionOwner(ctx, ws, owner); err == nil {
		t.Fatal("owner provision survived audit failure")
	}
	if _, err = s.Membership(ctx, ws, owner); !errors.Is(err, ErrForbidden) {
		t.Fatal("owner mutation was not rolled back", err)
	}
	if rows := auditRows(t, s, ws); len(rows) != 1 {
		t.Fatal("rollback left audit records")
	}
	var cursor int64
	if err = s.Pool.QueryRow(ctx, `SELECT sequence FROM audit_cursors WHERE workspace_id=$1`, ws).Scan(&cursor); err != nil || cursor != 1 {
		t.Fatal("rollback consumed audit sequence", err)
	}
}

func TestAuditPaginationAndConcurrentCommitOrder(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws := workspace(t, s)
	first, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Rollback(ctx)
	if err = appendAudit(ctx, first, ws, "workflow.created", "fixture", AuditDetails{}); err != nil {
		t.Fatal(err)
	}
	// An uncommitted lower sequence must block later allocation, not merely hide
	// a record which can appear behind a cursor already returned to the reader.
	secondCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	second, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Rollback(ctx)
	err = appendAudit(secondCtx, second, ws, "workflow.created", "later", AuditDetails{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("later sequence was not blocked behind earlier transaction", err)
	}
	if rows := auditRows(t, s, ws); len(rows) != 0 {
		t.Fatal("uncommitted audit was visible")
	}
	if err = first.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 104)
	for range 104 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.CreateWorkflow(ctx, ws, "Fixture", graph); results <- err }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	page := auditRows(t, s, ws)
	if len(page) != 100 {
		t.Fatal("unbounded audit page", len(page))
	}
	next, err := s.AuditRecords(ctx, ws, page[99].Sequence)
	if err != nil || len(next) != 5 {
		t.Fatal("cursor lost records", err)
	}
	for i, row := range append(page, next...) {
		if row.Sequence != int64(i+1) {
			t.Fatal("audit cursor gap")
		}
	}
	for _, after := range []int64{-1, 9007199254740992} {
		if _, err = s.AuditRecords(ctx, ws, after); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid cursor accepted")
		}
	}
}

func rejectAudit(t *testing.T, s *Store, ws string) {
	t.Helper()
	ctx := context.Background()
	// A workspace-specific failing audit trigger models storage failure without
	// changing any product validation or weakening append-only protection.
	_, err := s.Pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION fail_fixture_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id='%s' THEN RAISE EXCEPTION 'fixture audit unavailable'; END IF; RETURN NEW; END; $$; CREATE TRIGGER fail_fixture_audit BEFORE INSERT ON audit_records FOR EACH ROW EXECUTE FUNCTION fail_fixture_audit()`, ws))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := s.Pool.Exec(ctx, `DROP TRIGGER fail_fixture_audit ON audit_records; DROP FUNCTION fail_fixture_audit()`); err != nil {
			t.Error(err)
		}
	})
}

func TestAuditFailureRollsBackRunCredentialAndScheduleMutations(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	r, snap := executable(t, s)
	ws := r.WorkspaceID
	run, err := s.GetRun(ctx, ws, r.RunID)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := s.GetWorkflow(ctx, ws, run.WorkflowID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateWorkflow(ctx, ws, "Other", graph)
	if err != nil {
		t.Fatal(err)
	}
	owner := identity.Principal{Issuer: "https://fixture.example", Subject: "owner"}
	if err = s.ProvisionOwner(ctx, ws, owner); err != nil {
		t.Fatal(err)
	}
	box, _ := secrets.New(strings.Repeat("ab", 32), "fixture")
	credential, err := s.CreateCredential(ctx, ws, "Fixture", "bearer", "original-fixture-token", "example.com", box)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantCredential(ctx, ws, credential.ID, workflow.ID); err != nil {
		t.Fatal(err)
	}
	envelope, err := s.PrepareInvocation(ctx, r, snap.Plan.Steps[1], snap.InputRef)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkUncertain(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	baseline := len(auditRows(t, s, ws))
	rejectAudit(t, s, ws)
	if _, err = s.RequestCancel(ctx, ws, r.RunID); err == nil {
		t.Fatal("cancellation survived audit failure")
	}
	if got, e := s.GetRun(ctx, ws, r.RunID); e != nil || got.CancelRequested {
		t.Fatal("cancellation not rolled back", e)
	}
	if _, err = s.AdmitExecution(ctx, ws, workflow.ID, "second", []byte(`{}`)); err == nil {
		t.Fatal("admission survived audit failure")
	}
	if runs, e := s.Runs(ctx, ws, workflow.ID, ""); e != nil || len(runs) != 1 {
		t.Fatal("admission not rolled back", e)
	}
	if err = s.RotateCredential(ctx, ws, credential.ID, "replacement-fixture-token", box); err == nil {
		t.Fatal("rotation survived audit failure")
	}
	if value, e := s.ResolveCredential(ctx, ws, workflow.ID, credential.ID, "bearer", "example.com", box); e != nil || value != "original-fixture-token" {
		t.Fatal("rotation not rolled back")
	}
	if err = s.RevokeCredential(ctx, ws, credential.ID); err == nil {
		t.Fatal("revocation survived audit failure")
	}
	if _, e := s.ResolveCredential(ctx, ws, workflow.ID, credential.ID, "bearer", "example.com", box); e != nil {
		t.Fatal("revocation not rolled back")
	}
	if err = s.GrantCredential(ctx, ws, credential.ID, other.ID); err == nil {
		t.Fatal("grant survived audit failure")
	}
	if err = s.CredentialAllowed(ctx, ws, other.ID, credential.ID, "bearer"); !errors.Is(err, ErrNotFound) {
		t.Fatal("grant not rolled back", err)
	}
	spec := ScheduleSpec{Kind: "interval", IntervalSeconds: 60, Timezone: "UTC", Enabled: true, Input: []byte(`{}`)}
	if _, err = s.SaveSchedule(ctx, Schedule{WorkspaceID: ws, WorkflowID: workflow.ID, Spec: spec}, 0); err == nil {
		t.Fatal("schedule survived audit failure")
	}
	if schedules, e := s.Schedules(ctx, ws); e != nil || len(schedules) != 0 {
		t.Fatal("schedule not rolled back", e)
	}
	member := Membership{Issuer: owner.Issuer, Subject: "viewer", Role: "viewer"}
	if err = s.ChangeMembership(ctx, ws, owner, member, false); err == nil {
		t.Fatal("membership survived audit failure")
	}
	if _, err = s.Membership(ctx, ws, identity.Principal{Issuer: member.Issuer, Subject: member.Subject}); !errors.Is(err, ErrForbidden) {
		t.Fatal("membership not rolled back", err)
	}
	resolution := Resolution{WorkspaceID: ws, RunID: r.RunID, InvocationID: envelope.InvocationID, Action: "fail", Actor: "test-operator", Note: "fixture"}
	if _, err = s.ResolveUncertain(ctx, resolution, nil); err == nil {
		t.Fatal("resolution survived audit failure")
	}
	if _, err = s.Resolution(ctx, envelope); !errors.Is(err, ErrNotFound) {
		t.Fatal("resolution not rolled back", err)
	}
	bundle, err := definition.ExportPortable("Fixture", graph, s.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportWorkflow(ctx, ws, bundle, nil); err == nil {
		t.Fatal("import survived audit failure")
	}
	if workflows, e := s.Workflows(ctx, ws, ""); e != nil || len(workflows) != 2 {
		t.Fatal("import not rolled back", e)
	}
	if len(auditRows(t, s, ws)) != baseline {
		t.Fatal("failed writes left audit records")
	}
}
