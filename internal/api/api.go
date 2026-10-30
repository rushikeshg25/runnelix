// Package api exposes a loopback development API. Workspace routes are not authentication.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/identity"
	"github.com/rushikeshg25/runnelix/internal/secrets"
	"github.com/rushikeshg25/runnelix/internal/store"
)

type API struct {
	Identity      *identity.Verifier
	Browser       *identity.Browser
	Store         *store.Store
	Box           *secrets.Box
	BrowserOrigin string
}
type requestKey struct{}
type Error struct {
	Code      string             `json:"code"`
	Message   string             `json:"message"`
	RequestID string             `json:"requestId"`
	Issues    []definition.Issue `json:"issues,omitempty"`
}

func (a API) Handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(pattern string, permission store.Permission, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, a.authorize(permission, handler))
	}
	handle("GET /v1/workspaces/{workspace}/operations", store.Manage, a.operationalMetrics)
	handle("GET /v1/workspaces/{workspace}/audit-records", store.Manage, a.auditRecords)
	handle("GET /v1/workspaces/{workspace}/memberships", store.Manage, a.memberships)
	handle("PUT /v1/workspaces/{workspace}/memberships", store.Manage, a.changeMembership)
	handle("DELETE /v1/workspaces/{workspace}/memberships", store.Manage, a.changeMembership)
	handle("GET /v1/auth/login", publicRoute, a.login)
	handle("GET /v1/auth/callback", publicRoute, a.callback)
	handle("GET /v1/auth/session", publicRoute, a.session)
	handle("POST /v1/auth/logout", signedIn, a.logout)
	handle("GET /v1/identity", signedIn, a.identity)
	handle("GET /health", publicRoute, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if a.Store.CheckSchema(ctx) != nil {
			respond(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		mode := "development"
		if a.Identity != nil {
			mode = "oidc"
		}
		respond(w, 200, map[string]string{"status": "ready", "mode": mode})
	})
	handle("GET /v1/node-types", signedIn, func(w http.ResponseWriter, r *http.Request) { respond(w, 200, a.Store.Catalog) })
	handle("POST /v1/workspaces/{workspace}/workflows", store.Edit, a.create)
	handle("POST /v1/workspaces/{workspace}/workflows/import", store.Edit, a.importWorkflow)
	handle("GET /v1/workspaces/{workspace}/workflows/{workflow}/export", store.Read, a.exportWorkflow)
	handle("GET /v1/workspaces/{workspace}/workflows/{workflow}", store.Read, a.read)
	handle("PUT /v1/workspaces/{workspace}/workflows/{workflow}/draft", store.Edit, a.save)
	handle("POST /v1/workspaces/{workspace}/workflows/{workflow}/validate", store.Edit, a.validate)
	handle("POST /v1/workspaces/{workspace}/workflows/{workflow}/publish", store.Publish, a.publish)
	handle("GET /v1/workspaces/{workspace}/workflows/{workflow}/versions/{version}", store.Read, a.version)
	handle("POST /v1/workspaces/{workspace}/workflows/{workflow}/runs", store.Execute, a.admit)
	handle("GET /v1/workspaces/{workspace}/runs/{run}", store.Read, a.run)

	handle("GET /v1/workspaces/{workspace}/workflows", store.Read, a.list)
	handle("GET /v1/workspaces/{workspace}/workflows/{workflow}/runs", store.Read, a.runs)
	handle("POST /v1/workspaces/{workspace}/workflows/{workflow}/execute", store.Execute, a.execute)
	handle("POST /v1/workspaces/{workspace}/runs/{run}/cancel", store.Execute, a.cancel)
	handle("POST /v1/workspaces/{workspace}/runs/{run}/invocations/{invocation}/resolve", store.Execute, a.resolve)
	handle("GET /v1/workspaces/{workspace}/runs/{run}/attempts", store.DataRead, a.attempts)
	handle("GET /v1/workspaces/{workspace}/runs/{run}/events", store.DataRead, a.events)
	handle("GET /v1/workspaces/{workspace}/runs/{run}/payloads/{ref}", store.DataRead, a.payload)
	handle("POST /v1/mapping-preview", signedIn, a.preview)
	handle("GET /v1/workspaces/{workspace}/schedules", store.DataRead, a.schedules)
	handle("GET /v1/workspaces/{workspace}/required-artifacts", store.Read, a.requiredArtifacts)
	handle("POST /v1/workspaces/{workspace}/schedules", store.Execute|store.DataRead, a.saveSchedule)
	handle("PUT /v1/workspaces/{workspace}/schedules/{schedule}", store.Execute|store.DataRead, a.saveSchedule)
	handle("GET /v1/workspaces/{workspace}/credentials", store.Manage, a.credentials)
	handle("POST /v1/workspaces/{workspace}/credentials", store.Manage, a.createCredential)
	handle("PUT /v1/workspaces/{workspace}/credentials/{credential}/value", store.Manage, a.rotateCredential)
	handle("POST /v1/workspaces/{workspace}/credentials/{credential}/revoke", store.Manage, a.revokeCredential)
	handle("PUT /v1/workspaces/{workspace}/credentials/{credential}/grants/{workflow}", store.Manage, a.grant)
	handle("POST /v1/workspaces/{workspace}/webhooks/{workflow}/{credential}", store.Execute, a.webhook)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := "request_" + uuid.NewString()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Browser origins are not trusted by this unauthenticated development process.
		if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != a.BrowserOrigin {
			respond(w, 403, Error{"origin_forbidden", "Browser origin is not allowed", id, nil})
			return
		}
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			respond(w, 403, Error{"host_forbidden", "Literal loopback host is required", id, nil})
			return
		}
		ctx, cancel := context.WithTimeout(context.WithValue(r.Context(), requestKey{}, id), 10*time.Second)
		defer cancel()
		authenticated, ok := a.authenticate(w, r.WithContext(ctx))
		if !ok {
			return
		}
		actor := "local-development"
		if principal, ok := authenticated.Context().Value(principalKey{}).(identity.Principal); ok {
			actor = store.PrincipalActor(principal)
		}
		authenticated = authenticated.WithContext(store.WithAuditContext(authenticated.Context(), actor, id))
		mux.ServeHTTP(w, authenticated)
	})
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := 500, "internal", "Request could not be completed"
	var issues []definition.Issue
	var invalid *definition.Invalid
	switch {
	case errors.Is(err, store.ErrForbidden):
		status, code, msg = 403, "forbidden", "Workspace permission is required"
	case errors.Is(err, store.ErrOverlap):
		status, code, msg = 409, "workflow_active", "Scheduled workflow already has an active run"
	case errors.Is(err, store.ErrLimit):
		status, code, msg = 429, "limit_exceeded", "Workspace or run limit reached"
	case errors.As(err, &invalid):
		status, code, msg = 422, "invalid_workflow", "Workflow validation failed"
		issues = invalid.Issues
	case errors.Is(err, store.ErrExpired):
		status, code, msg = http.StatusGone, "data_expired", "Run data expired under retention policy"
	case errors.Is(err, store.ErrNotFound):
		status, code, msg = 404, "not_found", "Record not found"
	case errors.Is(err, store.ErrConflict):
		status, code, msg = 409, "conflict", "Revision, pinned version, or idempotency key conflicts"
	case errors.Is(err, store.ErrUnpublished):
		status, code, msg = 409, "unpublished", "Publish the workflow before admission"
	case errors.Is(err, store.ErrInvalid):
		status, code, msg = 400, "invalid_request", "Invalid request fields or bounds"
	}
	id, _ := r.Context().Value(requestKey{}).(string)
	respond(w, status, Error{code, msg, id, issues})
}
func body(w http.ResponseWriter, r *http.Request, target any) error {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return store.ErrInvalid
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, definition.MaxBytes))
	if err != nil {
		return store.ErrInvalid
	}
	if err = definition.Decode(data, target); err != nil {
		return store.ErrInvalid
	}
	return nil
}
func revision(r *http.Request) (int64, error) {
	s := r.Header.Get("If-Match")
	if len(s) < 3 || s[0] != '"' || s[len(s)-1] != '"' {
		return 0, store.ErrInvalid
	}
	n, err := strconv.ParseInt(s[1:len(s)-1], 10, 64)
	if err != nil || n < 1 || n > 9007199254740991 {
		return 0, store.ErrInvalid
	}
	return n, nil
}
func (a API) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string          `json:"name"`
		Graph json.RawMessage `json:"graph"`
	}
	if err := body(w, r, &req); err != nil {
		fail(w, r, err)
		return
	}
	value, err := a.Store.CreateWorkflow(r.Context(), r.PathValue("workspace"), req.Name, req.Graph)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(value.Revision, 10)))
	respond(w, 201, value)
}
func (a API) read(w http.ResponseWriter, r *http.Request) {
	value, err := a.Store.GetWorkflow(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"))
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(value.Revision, 10)))
	respond(w, 200, value)
}
func (a API) save(w http.ResponseWriter, r *http.Request) {
	rev, err := revision(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	var req struct {
		Graph json.RawMessage `json:"graph"`
	}
	if err = body(w, r, &req); err != nil {
		fail(w, r, err)
		return
	}
	value, err := a.Store.SaveDraft(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"), rev, req.Graph)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(value.Revision, 10)))
	respond(w, 200, value)
}
func (a API) validate(w http.ResponseWriter, r *http.Request) {
	draft, err := a.Store.GetWorkflow(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"))
	if err != nil {
		fail(w, r, err)
		return
	}
	_, plan, digest, err := definition.Compile(draft.Graph, a.Store.Catalog)
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, map[string]any{"revision": draft.Revision, "digest": digest, "plan": plan})
}
func (a API) publish(w http.ResponseWriter, r *http.Request) {
	rev, err := revision(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	value, err := a.Store.Publish(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"), rev)
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, value)
}
func (a API) version(w http.ResponseWriter, r *http.Request) {
	value, err := a.Store.GetVersion(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"), r.PathValue("version"))
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, value)
}
func (a API) admit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input json.RawMessage `json:"input"`
	}
	if err := body(w, r, &req); err != nil {
		fail(w, r, err)
		return
	}
	value, err := a.Store.Admit(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"), r.Header.Get("Idempotency-Key"), req.Input)
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 202, value)
}
func (a API) run(w http.ResponseWriter, r *http.Request) {
	value, err := a.Store.GetRun(r.Context(), r.PathValue("workspace"), r.PathValue("run"))
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, value)
}
