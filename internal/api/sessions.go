package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/rushikeshg25/runnelix/internal/identity"
	"github.com/rushikeshg25/runnelix/internal/store"
)

type sessionKey struct{}
type browserSession struct {
	Hash, CSRF string
	Expires    time.Time
}

func randomToken() string {
	var raw [32]byte
	// crypto/rand.Read terminates the process if the system entropy source fails.
	_, _ = rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:])
}
func (a API) cookieName(kind string) string {
	if a.Browser != nil && a.Browser.Secure {
		return "__Host-runnelix_" + kind
	}
	return "runnelix_" + kind
}
func (a API) cookie(w http.ResponseWriter, kind, value string, expires time.Time) {
	age := int(time.Until(expires).Seconds())
	if value == "" {
		age = -1
	}
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(kind), Value: value, Path: "/", HttpOnly: true, Secure: a.Browser.Secure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: age})
}
func (a API) readCookie(r *http.Request, kind string) (string, error) {
	cookies := r.CookiesNamed(a.cookieName(kind))
	if len(cookies) != 1 || len(cookies[0].Value) != 43 {
		return "", identity.ErrUnauthenticated
	}
	if raw, err := base64.RawURLEncoding.DecodeString(cookies[0].Value); err != nil || len(raw) != 32 {
		return "", identity.ErrUnauthenticated
	}
	return cookies[0].Value, nil
}
func (a API) login(w http.ResponseWriter, r *http.Request) {
	if a.Browser == nil {
		fail(w, r, store.ErrForbidden)
		return
	}
	state, nonce, verifier := randomToken(), randomToken(), randomToken()
	err := a.Store.CreateLogin(r.Context(), store.LoginChallenge{StateHash: store.TokenHash(state), Verifier: verifier, Nonce: nonce, Issuer: a.Browser.Issuer(), ClientID: a.Browser.ClientID()})
	if err != nil {
		fail(w, r, err)
		return
	}
	a.cookie(w, "login", state, time.Now().Add(10*time.Minute))
	http.Redirect(w, r, a.Browser.LoginURL(state, nonce, verifier), http.StatusFound)
}
func (a API) callback(w http.ResponseWriter, r *http.Request) {
	if a.Browser == nil {
		fail(w, r, store.ErrForbidden)
		return
	}
	failed := func() { http.Redirect(w, r, a.Browser.Origin+"/?login=failed", http.StatusSeeOther) }
	state, err := a.readCookie(r, "login")
	a.cookie(w, "login", "", time.Unix(1, 0))
	query := r.URL.Query()
	if err != nil || len(query["state"]) != 1 || subtle.ConstantTimeCompare([]byte(state), []byte(query.Get("state"))) != 1 {
		failed()
		return
	}
	challenge, err := a.Store.ConsumeLogin(r.Context(), store.TokenHash(state), a.Browser.Issuer(), a.Browser.ClientID())
	if err != nil || len(query["code"]) != 1 || query.Get("error") != "" {
		failed()
		return
	}
	principal, expires, err := a.Browser.Exchange(r.Context(), query.Get("code"), challenge.Verifier, challenge.Nonce)
	if err != nil {
		failed()
		return
	}
	previous, _ := a.readCookie(r, "session")
	raw := randomToken()
	if err = a.Store.CreateSession(r.Context(), store.TokenHash(raw), store.TokenHash(previous), principal, a.Browser.ClientID(), expires); err != nil {
		failed()
		return
	}
	a.cookie(w, "session", raw, expires)
	http.Redirect(w, r, a.Browser.Origin+"/", http.StatusSeeOther)
}
func (a API) session(w http.ResponseWriter, r *http.Request) {
	if a.Identity == nil {
		respond(w, 200, map[string]any{"mode": "development", "authenticated": true})
		return
	}
	principal, ok := r.Context().Value(principalKey{}).(identity.Principal)
	result := map[string]any{"mode": "oidc", "authenticated": ok, "loginEnabled": a.Browser != nil}
	if ok {
		result["principal"] = principal
	}
	if session, ok := r.Context().Value(sessionKey{}).(browserSession); ok {
		result["csrfToken"] = session.CSRF
		result["expiresAt"] = session.Expires
	}
	respond(w, 200, result)
}
func (a API) logout(w http.ResponseWriter, r *http.Request) {
	session, ok := r.Context().Value(sessionKey{}).(browserSession)
	if !ok || a.Browser == nil {
		fail(w, r, store.ErrForbidden)
		return
	}
	if err := a.Store.DeleteSession(r.Context(), session.Hash); err != nil {
		fail(w, r, err)
		return
	}
	a.cookie(w, "session", "", time.Unix(1, 0))
	respond(w, 200, map[string]bool{"signedOut": true})
}
func (a API) authenticateSession(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	raw, err := a.readCookie(r, "session")
	if err != nil {
		if r.Method == "GET" && r.URL.Path == "/v1/auth/session" {
			a.cookie(w, "session", "", time.Unix(1, 0))
			return r, true
		}
		a.unauthenticated(w, r)
		return r, false
	}
	principal, expires, err := a.Store.Session(r.Context(), store.TokenHash(raw), a.Browser.Issuer(), a.Browser.ClientID())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			a.cookie(w, "session", "", time.Unix(1, 0))
			if r.Method == "GET" && r.URL.Path == "/v1/auth/session" {
				return r, true
			}
			a.unauthenticated(w, r)
		} else {
			fail(w, r, err)
		}
		return r, false
	}
	csrf := store.TokenHash("csrf:" + raw)
	if r.Method != "GET" && r.Method != "HEAD" {
		if r.Header.Get("Origin") != a.Browser.Origin || len(r.Header.Values("X-CSRF-Token")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) != 1 {
			id, _ := r.Context().Value(requestKey{}).(string)
			respond(w, 403, Error{"csrf_failed", "Reload the page before trying this action again", id, nil})
			return r, false
		}
	}
	ctx := context.WithValue(r.Context(), principalKey{}, principal)
	ctx = context.WithValue(ctx, sessionKey{}, browserSession{Hash: store.TokenHash(raw), CSRF: csrf, Expires: expires})
	return r.WithContext(ctx), true
}
