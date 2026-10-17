// Package nodeservice is the authenticated worker data plane. Never expose it publicly.
package nodeservice

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/secrets"
	"github.com/rushikeshg25/runnelix/internal/store"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Service struct {
	Store  *store.Store
	Token  string
	Box    *secrets.Box
	Egress Egress
}
type taskRequest struct {
	Envelope  definition.Envelope `json:"envelope"`
	Request   Request             `json:"request,omitempty"`
	Attempt   int                 `json:"attempt,omitempty"`
	Status    string              `json:"status,omitempty"`
	Output    any                 `json:"output,omitempty"`
	ErrorCode string              `json:"errorCode,omitempty"`
}

func (s Service) Handler() (http.Handler, error) {
	if len(s.Token) < 32 {
		return nil, errors.New("worker token must contain at least 32 characters")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "POST" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), []byte(s.Token)) != 1 {
			failure(w, 401, "Unauthorized", false)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 24000))
		if err != nil {
			failure(w, 400, "InvalidInput", false)
			return
		}
		var task taskRequest
		if definition.Decode(raw, &task) != nil {
			failure(w, 400, "InvalidInput", false)
			return
		}
		data, _ := json.Marshal(task.Envelope)
		if _, err = definition.ParseEnvelope(data); err != nil {
			failure(w, 400, "InvalidInput", false)
			return
		}
		if s.Store.Invocation(r.Context(), task.Envelope) != nil {
			failure(w, 403, "InvalidInput", false)
			return
		}
		run, err := s.Store.GetRun(r.Context(), task.Envelope.WorkspaceID, task.Envelope.RunID)
		if err != nil {
			failure(w, 503, "StorageUnavailable", true)
			return
		}
		// Completion evidence remains reportable after cancellation; new provider requests do not.
		if r.URL.Path != "/internal/node/complete" && (run.CancelRequested || run.State == "cancelled" || run.State == "failed" || run.State == "succeeded") {
			failure(w, 409, "Cancelled", false)
			return
		}
		switch r.URL.Path {
		case "/internal/node/input":
			input, err := s.Store.Payload(r.Context(), task.Envelope.WorkspaceID, task.Envelope.RunID, task.Envelope.InputRef)
			if err != nil {
				failure(w, 404, "PayloadUnavailable", false)
				return
			}
			config, err := s.Store.Payload(r.Context(), task.Envelope.WorkspaceID, task.Envelope.RunID, task.Envelope.ConfigRef)
			if err != nil {
				failure(w, 404, "PayloadUnavailable", false)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"input": input, "config": config})
		case "/internal/node/request":
			raw, err := s.Store.Payload(r.Context(), task.Envelope.WorkspaceID, task.Envelope.RunID, task.Envelope.ConfigRef)
			if err != nil {
				failure(w, 404, "PayloadUnavailable", false)
				return
			}
			var config map[string]any
			if json.Unmarshal(raw, &config) != nil {
				failure(w, 400, "InvalidInput", false)
				return
			}
			if !approvedRequest(task.Envelope.NodeType, config, task.Request) {
				failure(w, 403, "EgressDenied", false)
				return
			}
			destination, err := url.Parse(task.Request.URL)
			if err != nil {
				failure(w, 400, "InvalidInput", false)
				return
			}
			credential := ""
			if ref, ok := config["credentialRef"].(string); ok && ref != "" {
				credential, err = s.Store.ResolveCredential(r.Context(), task.Envelope.WorkspaceID, run.WorkflowID, ref, "bearer", strings.ToLower(destination.Hostname()), s.Box)
				if err != nil {
					failure(w, 403, "CredentialUnavailable", false)
					return
				}
			}
			result, err := s.Egress.Do(r.Context(), task.Request, credential)
			if err != nil {
				var provider *ProviderError
				if errors.As(err, &provider) {
					failure(w, 422, provider.Code, provider.Retryable)
				} else {
					failure(w, 503, "ProviderNetwork", true)
				}
				return
			}
			_ = json.NewEncoder(w).Encode(result)
		case "/internal/node/complete":
			if !allowedCode(task.ErrorCode) {
				failure(w, 400, "InvalidInput", false)
				return
			}
			ref := ""
			if task.Status == "succeeded" {
				ref, err = s.Store.PutPayload(r.Context(), task.Envelope.WorkspaceID, task.Envelope.RunID, task.Output)
				if err != nil {
					failure(w, 400, "InvalidOutput", false)
					return
				}
			}
			if err = s.Store.RecordAttempt(r.Context(), task.Envelope, task.Attempt, task.Status, ref, task.ErrorCode); err != nil {
				failure(w, 409, "InvalidResult", false)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"outputRef": ref, "attempt": task.Attempt})
		default:
			failure(w, 404, "NotFound", false)
		}
	}), nil
}
func approvedRequest(nodeType string, config map[string]any, request Request) bool {
	method := map[string]string{"http.get": "GET", "http.post": "POST"}[nodeType]
	target, ok := config["url"].(string)
	return ok && method != "" && method == request.Method && target == request.URL
}
func allowedCode(s string) bool {
	switch s {
	case "", "InvalidInput", "VersionMismatch", "PayloadUnavailable", "InvalidOutput", "ProviderTransient", "ProviderNetwork", "ProviderRejected", "EgressDenied", "CredentialUnavailable", "Cancelled":
		return true
	}
	return false
}
func failure(w http.ResponseWriter, status int, code string, retryable bool) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "retryable": retryable})
}
