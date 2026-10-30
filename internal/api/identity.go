package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/rushikeshg25/runnelix/internal/identity"
)

type principalKey struct{}

func (a API) identity(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalKey{}).(identity.Principal)
	if !ok {
		respond(w, 200, map[string]string{"mode": "development"})
		return
	}
	respond(w, 200, map[string]any{"mode": "oidc", "principal": principal})
}
func (a API) authenticate(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if a.Identity == nil || r.URL.Path == "/health" {
		return r, true
	}
	headers := r.Header.Values("Authorization")
	if len(headers) != 1 || !strings.HasPrefix(headers[0], "Bearer ") {
		a.unauthenticated(w, r)
		return r, false
	}
	principal, err := a.Identity.Verify(r.Context(), strings.TrimPrefix(headers[0], "Bearer "))
	if err != nil {
		a.unauthenticated(w, r)
		return r, false
	}
	// This identity-only slice cannot grant workspace privileges. Keep all routes
	// closed except identity inspection until server-side memberships are shipped.
	if r.Method != "GET" || r.URL.Path != "/v1/identity" {
		id, _ := r.Context().Value(requestKey{}).(string)
		respond(w, 403, Error{"authorization_unavailable", "Workspace authorization is not configured", id, nil})
		return r, false
	}
	return r.WithContext(context.WithValue(r.Context(), principalKey{}, principal)), true
}
func (a API) unauthenticated(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="runnelix"`)
	id, _ := r.Context().Value(requestKey{}).(string)
	respond(w, 401, Error{"unauthenticated", "A valid identity token is required", id, nil})
}
