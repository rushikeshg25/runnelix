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
	"github.com/rushikeshg25/runnelix/internal/secrets"
	"github.com/rushikeshg25/runnelix/internal/store"
)

type API struct {
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
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if a.Store.Pool.Ping(ctx) != nil {
			respond(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		respond(w, 200, map[string]string{"status": "ready", "mode": "development"})
	})
	mux.HandleFunc("GET /v1/node-types", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, a.Store.Catalog) })
	mux.HandleFunc("POST /v1/workspaces/{workspace}/workflows", a.create)
	mux.HandleFunc("GET /v1/workspaces/{workspace}/workflows/{workflow}", a.read)
	mux.HandleFunc("PUT /v1/workspaces/{workspace}/workflows/{workflow}/draft", a.save)
	mux.HandleFunc("POST /v1/workspaces/{workspace}/workflows/{workflow}/validate", a.validate)
	mux.HandleFunc("POST /v1/workspaces/{workspace}/workflows/{workflow}/publish", a.publish)
	mux.HandleFunc("GET /v1/workspaces/{workspace}/workflows/{workflow}/versions/{version}", a.version)
	mux.HandleFunc("POST /v1/workspaces/{workspace}/workflows/{workflow}/runs", a.admit)
	mux.HandleFunc("GET /v1/workspaces/{workspace}/runs/{run}", a.run)

	mux.HandleFunc("GET /v1/workspaces/{workspace}/workflows", a.list)
	mux.HandleFunc("GET /v1/workspaces/{workspace}/workflows/{workflow}/runs", a.runs)
	mux.HandleFunc("POST /v1/workspaces/{workspace}/workflows/{workflow}/execute", a.execute)
	mux.HandleFunc("POST /v1/workspaces/{workspace}/runs/{run}/cancel", a.cancel)
	mux.HandleFunc("POST /v1/workspaces/{workspace}/runs/{run}/invocations/{invocation}/resolve", a.resolve)
	mux.HandleFunc("GET /v1/workspaces/{workspace}/runs/{run}/attempts", a.attempts)
	mux.HandleFunc("GET /v1/workspaces/{workspace}/runs/{run}/events", a.events)
	mux.HandleFunc("GET /v1/workspaces/{workspace}/runs/{run}/payloads/{ref}", a.payload)
	mux.HandleFunc("POST /v1/mapping-preview", a.preview)
	mux.HandleFunc("GET /v1/workspaces/{workspace}/credentials", a.credentials)
	mux.HandleFunc("POST /v1/workspaces/{workspace}/credentials", a.createCredential)
	mux.HandleFunc("PUT /v1/workspaces/{workspace}/credentials/{credential}/grants/{workflow}", a.grant)
	mux.HandleFunc("POST /v1/workspaces/{workspace}/webhooks/{workflow}/{credential}", a.webhook)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := "request_" + uuid.NewString()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
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
		mux.ServeHTTP(w, r.WithContext(ctx))
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
	case errors.As(err, &invalid):
		status, code, msg = 422, "invalid_workflow", "Workflow validation failed"
		issues = invalid.Issues
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
