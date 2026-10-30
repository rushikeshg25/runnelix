package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/rushikeshg25/runnelix/internal/identity"
	"github.com/rushikeshg25/runnelix/internal/store"
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
	if r.Method == "GET" && (r.URL.Path == "/v1/auth/login" || r.URL.Path == "/v1/auth/callback") {
		return r, true
	}
	headers := r.Header.Values("Authorization")
	cookies := r.CookiesNamed(a.cookieName("session"))
	if len(headers) > 0 && len(cookies) > 0 {
		a.unauthenticated(w, r)
		return r, false
	}
	if len(headers) == 0 && len(cookies) == 0 && r.Method == "GET" && r.URL.Path == "/v1/auth/session" {
		return r, true
	}
	if len(headers) == 0 && a.Browser != nil {
		return a.authenticateSession(w, r)
	}
	if len(headers) != 1 || !strings.HasPrefix(headers[0], "Bearer ") {
		a.unauthenticated(w, r)
		return r, false
	}
	principal, err := a.Identity.Verify(r.Context(), strings.TrimPrefix(headers[0], "Bearer "))
	if err != nil {
		a.unauthenticated(w, r)
		return r, false
	}
	return r.WithContext(context.WithValue(r.Context(), principalKey{}, principal)), true
}
func (a API) unauthenticated(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="runnelix"`)
	id, _ := r.Context().Value(requestKey{}).(string)
	respond(w, 401, Error{"unauthenticated", "A valid identity token is required", id, nil})
}

const (
	signedIn    store.Permission = 1 << 6
	publicRoute store.Permission = 1 << 7
)

func (a API) permission(r *http.Request, required store.Permission) error {
	if a.Identity == nil {
		return nil
	}
	principal, ok := r.Context().Value(principalKey{}).(identity.Principal)
	if !ok {
		return store.ErrForbidden
	}
	if session, ok := r.Context().Value(sessionKey{}).(browserSession); ok {
		if a.Browser == nil {
			return store.ErrForbidden
		}
		if _, _, err := a.Store.Session(r.Context(), session.Hash, a.Browser.Issuer(), a.Browser.ClientID()); err != nil {
			return store.ErrForbidden
		}
	}
	workspace := r.PathValue("workspace")
	if required == signedIn && workspace == "" {
		return nil
	}
	if workspace == "" {
		return store.ErrForbidden
	}
	member, err := a.Store.Membership(r.Context(), workspace, principal)
	if err != nil {
		return err
	}
	if !member.Allows(required) {
		return store.ErrForbidden
	}
	return nil
}
func (a API) authorize(required store.Permission, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if required != publicRoute {
			if err := a.permission(r, required); err != nil {
				fail(w, r, err)
				return
			}
		}
		next(w, r)
	}
}
func (a API) memberships(w http.ResponseWriter, r *http.Request) {
	members, err := a.Store.Memberships(r.Context(), r.PathValue("workspace"))
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, members)
}
func (a API) changeMembership(w http.ResponseWriter, r *http.Request) {
	// Development mode cannot fabricate an owner identity for membership changes.
	principal, ok := r.Context().Value(principalKey{}).(identity.Principal)
	if !ok {
		fail(w, r, store.ErrForbidden)
		return
	}
	var member store.Membership
	if r.Method == "DELETE" {
		var target identity.Principal
		if err := body(w, r, &target); err != nil {
			fail(w, r, err)
			return
		}
		member.Issuer, member.Subject = target.Issuer, target.Subject
	} else if err := body(w, r, &member); err != nil {
		fail(w, r, err)
		return
	}
	if err := a.Store.ChangeMembership(r.Context(), r.PathValue("workspace"), principal, member, r.Method == "DELETE"); err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, map[string]bool{"updated": true})
}
