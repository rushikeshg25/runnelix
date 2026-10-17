package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/rushikeshg25/runnelix/internal/mapping"
	"github.com/rushikeshg25/runnelix/internal/store"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (a API) list(w http.ResponseWriter, r *http.Request) {
	v, e := a.Store.Workflows(r.Context(), r.PathValue("workspace"), r.URL.Query().Get("after"))
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 200, v)
}
func (a API) runs(w http.ResponseWriter, r *http.Request) {
	v, e := a.Store.Runs(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"), r.URL.Query().Get("after"))
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 200, v)
}
func (a API) execute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input json.RawMessage `json:"input"`
	}
	if e := body(w, r, &req); e != nil {
		fail(w, r, e)
		return
	}
	v, e := a.Store.AdmitExecution(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"), r.Header.Get("Idempotency-Key"), req.Input)
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 202, v)
}
func (a API) cancel(w http.ResponseWriter, r *http.Request) {
	v, e := a.Store.RequestCancel(r.Context(), r.PathValue("workspace"), r.PathValue("run"))
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 202, v)
}
func (a API) attempts(w http.ResponseWriter, r *http.Request) {
	v, e := a.Store.Attempts(r.Context(), r.PathValue("workspace"), r.PathValue("run"))
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 200, v)
}
func (a API) payload(w http.ResponseWriter, r *http.Request) {
	v, e := a.Store.Payload(r.Context(), r.PathValue("workspace"), r.PathValue("run"), r.PathValue("ref"))
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 200, v)
}
func (a API) events(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("after")
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		cursor = v
	}
	if cursor == "" {
		cursor = "0"
	}
	after, e := strconv.ParseInt(cursor, 10, 64)
	if e != nil || after < 0 {
		fail(w, r, store.ErrInvalid)
		return
	}
	stream := r.Header.Get("Accept") == "text/event-stream"
	events, e := a.Store.Events(r.Context(), r.PathValue("workspace"), r.PathValue("run"), after)
	if e != nil {
		fail(w, r, e)
		return
	}
	if !stream {
		respond(w, 200, events)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, r, store.ErrInvalid)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	// Short bounded streams reconnect with a durable cursor; no in-memory event ownership.
	for {
		for _, event := range events {
			raw, _ := json.Marshal(event)
			if _, e = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.Sequence, raw); e != nil {
				return
			}
			after = event.Sequence
		}
		_, _ = io.WriteString(w, ": heartbeat\n\n")
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
		events, e = a.Store.Events(r.Context(), r.PathValue("workspace"), r.PathValue("run"), after)
		if e != nil {
			return
		}
	}
}
func (a API) preview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Expression string `json:"expression"`
		Input      any    `json:"input"`
	}
	if e := body(w, r, &req); e != nil {
		fail(w, r, e)
		return
	}
	value, e := mapping.Evaluate(r.Context(), req.Expression, req.Input)
	if e != nil {
		fail(w, r, store.ErrInvalid)
		return
	}
	respond(w, 200, map[string]any{"value": value})
}
func (a API) credentials(w http.ResponseWriter, r *http.Request) {
	v, e := a.Store.Credentials(r.Context(), r.PathValue("workspace"))
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 200, v)
}
func (a API) createCredential(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		Value       string `json:"value"`
		Destination string `json:"destination"`
	}
	if e := body(w, r, &req); e != nil {
		fail(w, r, e)
		return
	}
	v, e := a.Store.CreateCredential(r.Context(), r.PathValue("workspace"), req.Name, req.Kind, req.Value, req.Destination, a.Box)
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 201, v)
}
func (a API) grant(w http.ResponseWriter, r *http.Request) {
	e := a.Store.GrantCredential(r.Context(), r.PathValue("workspace"), r.PathValue("credential"), r.PathValue("workflow"))
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 200, map[string]bool{"granted": true})
}
func validSignature(secret, timestamp, key, signature string, raw []byte, now time.Time) bool {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds < now.Unix()-300 || seconds > now.Unix()+30 {
		return false
	}
	actual, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + key + "."))
	_, _ = mac.Write(raw)
	return hmac.Equal(actual, mac.Sum(nil))
}
func (a API) webhook(w http.ResponseWriter, r *http.Request) {
	secret, e := a.Store.ResolveCredential(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"), r.PathValue("credential"), "webhook-hmac", "", a.Box)
	if e != nil {
		respond(w, 401, map[string]string{"code": "invalid_signature"})
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	key := r.Header.Get("Idempotency-Key")
	if e != nil || !validSignature(secret, r.Header.Get("X-Runnelix-Timestamp"), key, r.Header.Get("X-Runnelix-Signature"), raw, time.Now()) {
		respond(w, 401, map[string]string{"code": "invalid_signature"})
		return
	}
	v, e := a.Store.AdmitWebhook(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"), key, raw)
	if e != nil {
		fail(w, r, e)
		return
	}
	respond(w, 202, v)
}

func (a API) resolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		Output any    `json:"output"`
		Note   string `json:"note"`
	}
	if err := body(w, r, &req); err != nil {
		fail(w, r, err)
		return
	}
	result, err := a.Store.ResolveUncertain(r.Context(), store.Resolution{WorkspaceID: r.PathValue("workspace"), RunID: r.PathValue("run"), InvocationID: r.PathValue("invocation"), Action: req.Action, Actor: "local-development-operator", Note: req.Note}, req.Output)
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 202, result)
}

func (a API) schedules(w http.ResponseWriter, r *http.Request) {
	result, err := a.Store.Schedules(r.Context(), r.PathValue("workspace"))
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, result)
}
func (a API) saveSchedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkflowID string             `json:"workflowId"`
		Spec       store.ScheduleSpec `json:"spec"`
	}
	if err := body(w, r, &req); err != nil {
		fail(w, r, err)
		return
	}
	var rev int64
	var err error
	if r.Method == "PUT" {
		rev, err = revision(r)
		if err != nil {
			fail(w, r, err)
			return
		}
	}
	result, err := a.Store.SaveSchedule(r.Context(), store.Schedule{WorkspaceID: r.PathValue("workspace"), ID: r.PathValue("schedule"), WorkflowID: req.WorkflowID, Spec: req.Spec}, rev)
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, result)
}
