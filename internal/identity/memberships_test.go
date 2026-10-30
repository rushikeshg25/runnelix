package identity_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rushikeshg25/runnelix/internal/api"
	"github.com/rushikeshg25/runnelix/internal/identity"
	"github.com/rushikeshg25/runnelix/internal/store"
)

func TestSignedWorkspacePermissions(t *testing.T) {
	dsn := os.Getenv("RUNNELIX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	ctx := context.Background()
	s, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	ws := "ws_" + uuid.NewString()
	if err = s.CreateWorkspace(ctx, ws, "Membership fixture"); err != nil {
		t.Fatal(err)
	}
	f := issuer(t)
	owner := identity.Principal{Issuer: f.server.URL, Subject: "owner"}
	if err = s.ProvisionOwner(ctx, ws, owner); err != nil {
		t.Fatal(err)
	}
	handler := api.API{Store: s, Identity: f.verifier(t)}.Handler()
	tokens := map[string]string{}
	for _, subject := range []string{"owner", "viewer", "editor", "operator", "outsider"} {
		tokens[subject] = f.token(t, map[string]any{"sub": subject})
	}
	call := func(subject, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8787"+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+tokens[subject])
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", `"1"`)
		r.Header.Set("Idempotency-Key", "auth-fixture")
		r.Header.Set("X-Request-ID", "forged-client-reference")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	base := "/v1/workspaces/" + ws
	set := func(subject, role string, publish, execute, data bool) {
		m := store.Membership{Issuer: f.server.URL, Subject: subject, Role: role, Publish: publish, Execute: execute, DataRead: data}
		raw, _ := json.Marshal(m)
		if w := call("owner", "PUT", base+"/memberships", string(raw)); w.Code != 200 {
			t.Fatalf("set member: %d %s", w.Code, w.Body.String())
		}
	}
	for _, role := range []string{"viewer", "editor", "operator"} {
		set(role, role, false, false, false)
	}
	// Every registered workspace route is exercised with an identity lacking membership.
	routes := []struct{ method, path string }{
		{"GET", "/operations"}, {"GET", "/audit-records"}, {"GET", "/memberships"}, {"PUT", "/memberships"}, {"DELETE", "/memberships"},
		{"GET", "/workflows"}, {"POST", "/workflows"}, {"POST", "/workflows/import"},
		{"GET", "/workflows/w"}, {"GET", "/workflows/w/export"}, {"PUT", "/workflows/w/draft"},
		{"POST", "/workflows/w/validate"}, {"POST", "/workflows/w/publish"}, {"GET", "/workflows/w/versions/v"},
		{"POST", "/workflows/w/runs"}, {"GET", "/workflows/w/runs"}, {"POST", "/workflows/w/execute"},
		{"GET", "/runs/r"}, {"POST", "/runs/r/cancel"}, {"POST", "/runs/r/invocations/i/resolve"},
		{"GET", "/runs/r/attempts"}, {"GET", "/runs/r/events"}, {"GET", "/runs/r/payloads/p"},
		{"GET", "/schedules"}, {"POST", "/schedules"}, {"PUT", "/schedules/s"}, {"GET", "/required-artifacts"},
		{"GET", "/credentials"}, {"POST", "/credentials"}, {"PUT", "/credentials/c/value"},
		{"POST", "/credentials/c/revoke"}, {"PUT", "/credentials/c/grants/w"}, {"POST", "/webhooks/w/c"},
	}
	for _, route := range routes {
		for _, prefix := range []string{base, "/v1/workspaces/not-a-membership"} {
			if w := call("outsider", route.method, prefix+route.path, "{}"); w.Code != 403 {
				t.Fatalf("unprotected %s %s: %d", route.method, route.path, w.Code)
			}
		}
	}
	for _, route := range routes {
		if w := call("owner", route.method, "/v1/workspaces/not-a-membership"+route.path, "{}"); w.Code != 403 {
			t.Fatalf("owner crossed workspace: %s %s %d", route.method, route.path, w.Code)
		}
	}
	for _, tc := range []struct {
		subject, method, path string
		status                int
	}{
		{"viewer", "GET", "/workflows", 200}, {"viewer", "POST", "/workflows", 403},
		{"viewer", "GET", "/runs/r/events", 403}, {"viewer", "GET", "/runs/r/payloads/p", 403},
		{"viewer", "GET", "/schedules", 403}, {"editor", "POST", "/workflows/w/publish", 403},
		{"editor", "POST", "/workflows/w/execute", 403}, {"operator", "POST", "/workflows", 403},
		{"operator", "POST", "/workflows/w/publish", 403}, {"operator", "POST", "/workflows/w/execute", 400},
		{"operator", "GET", "/credentials", 403}, {"editor", "PUT", "/memberships", 403},
		{"owner", "GET", "/operations", 200}, {"viewer", "GET", "/operations", 403}, {"editor", "GET", "/operations", 403}, {"operator", "GET", "/operations", 403}, {"owner", "GET", "/audit-records", 200}, {"viewer", "GET", "/audit-records", 403}, {"editor", "GET", "/audit-records", 403}, {"operator", "GET", "/audit-records", 403},
		{"owner", "GET", "/memberships", 200}, {"owner", "GET", "/credentials", 200},
	} {
		if w := call(tc.subject, tc.method, base+tc.path, "{}"); w.Code != tc.status {
			t.Fatalf("%+v got %d: %s", tc, w.Code, w.Body.String())
		}
	}
	created := call("editor", "POST", base+"/workflows", `{"name":"Authorized draft","graph":{"formatVersion":1,"nodes":[{"id":"start","type":"trigger.manual","version":1,"config":{}},{"id":"echo","type":"mock.echo","version":1,"config":{}}],"edges":[{"from":{"node":"start","port":"out"},"to":{"node":"echo","port":"in"}}]}}`)
	if created.Code != 201 {
		t.Fatalf("editor create %d %s", created.Code, created.Body.String())
	}

	var workflow store.Workflow
	if err = json.Unmarshal(created.Body.Bytes(), &workflow); err != nil {
		t.Fatal(err)
	}
	audit := call("owner", "GET", base+"/audit-records", "")
	var records []store.AuditRecord
	if audit.Code != 200 || json.Unmarshal(audit.Body.Bytes(), &records) != nil {
		t.Fatal("audit read failed")
	}
	found := false
	for _, record := range records {
		if record.Action == "workflow.created" && record.TargetID == workflow.ID {
			found = true
			if record.Actor != store.PrincipalActor(identity.Principal{Issuer: f.server.URL, Subject: "editor"}) || record.RequestID != created.Header().Get("X-Request-ID") || record.RequestID == "forged-client-reference" {
				t.Fatal("audit attribution was not server-derived")
			}
		}
	}
	if !found {
		t.Fatal("successful API write missing audit")
	}
	for _, cursor := range []string{"-1", "not-a-number", "9007199254740992"} {
		if w := call("owner", "GET", base+"/audit-records?after="+cursor, ""); w.Code != 400 {
			t.Fatal("invalid audit cursor accepted")
		}
	}
	if w := call("editor", "POST", base+"/workflows/import", `{"bindings":{"slot":"credential"}}`); w.Code != 403 {
		t.Fatalf("import grant bypass %d", w.Code)
	}
	set("editor", "editor", true, true, true)
	for _, route := range []struct{ method, path string }{{"POST", "/workflows/w/publish"}, {"POST", "/workflows/w/execute"}, {"GET", "/runs/r/events"}} {
		if w := call("editor", route.method, base+route.path, "{}"); w.Code == 403 || w.Code == 500 {
			t.Fatalf("explicit grant rejected: %d", w.Code)
		}
	}

	if w := call("editor", "POST", base+"/workflows/"+workflow.ID+"/publish", "{}"); w.Code != 200 {
		t.Fatalf("publish %d %s", w.Code, w.Body.String())
	}
	admitted := call("editor", "POST", base+"/workflows/"+workflow.ID+"/execute", `{"input":{"fixture":"private"}}`)
	if admitted.Code != 202 {
		t.Fatalf("execute %d %s", admitted.Code, admitted.Body.String())
	}
	var run store.Run
	if err = json.Unmarshal(admitted.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if w := call("viewer", "GET", base+"/runs/"+run.ID, ""); w.Code != 200 || strings.Contains(w.Body.String(), "private") {
		t.Fatal("run summary leaked input", w.Code)
	}
	// A live SSE connection must close after its data grant is revoked.
	server := httptest.NewServer(handler)
	defer server.Close()
	streamCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(streamCtx, "GET", server.URL+base+"/runs/"+run.ID+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+tokens["editor"])
	req.Header.Set("Accept", "text/event-stream")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	if _, err = reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	set("editor", "editor", true, true, false)
	if _, err = io.ReadAll(reader); err != nil {
		t.Fatalf("stream did not close after revocation: %v", err)
	}
	raw, _ := json.Marshal(identity.Principal{Issuer: f.server.URL, Subject: "editor"})
	if w := call("owner", "DELETE", base+"/memberships", string(raw)); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := call("editor", "GET", base+"/workflows", ""); w.Code != 403 {
		t.Fatal("revocation ignored", w.Code)
	}
	raw, _ = json.Marshal(owner)
	if w := call("owner", "DELETE", base+"/memberships", string(raw)); w.Code != 409 {
		t.Fatal("last owner removed", w.Code)
	}
}
