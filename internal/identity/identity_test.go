package identity_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rushikeshg25/runnelix/internal/api"
	"github.com/rushikeshg25/runnelix/internal/identity"
	"github.com/rushikeshg25/runnelix/internal/store"
)

type issuerFixture struct {
	server *httptest.Server
	key    *rsa.PrivateKey
}

func issuer(t *testing.T) issuerFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": server.URL, "jwks_uri": server.URL + "/keys?version=fixture", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "fixture", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return issuerFixture{server, key}
}
func (f issuerFixture) verifier(t *testing.T) *identity.Verifier {
	t.Helper()
	v, err := identity.New(context.Background(), identity.Config{Mode: "oidc", Issuer: f.server.URL, ClientID: "fixture-client", AllowLoopbackIssuer: true})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func (f issuerFixture) token(t *testing.T, changes map[string]any) string {
	t.Helper()
	claims := map[string]any{"iss": f.server.URL, "sub": "fixture-user", "aud": "fixture-client", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Add(-time.Minute).Unix()}
	for k, v := range changes {
		if v == nil {
			delete(claims, k)
		} else {
			claims[k] = v
		}
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "fixture"})
	payload, _ := json.Marshal(claims)
	data := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	hash := sha256.Sum256([]byte(data))
	signature, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return data + "." + base64.RawURLEncoding.EncodeToString(signature)
}
func TestOIDCConfigurationFailsClosed(t *testing.T) {
	for _, env := range []map[string]string{
		{"RUNNELIX_AUTH_MODE": "typo"},
		{"RUNNELIX_AUTH_MODE": "oidc"},
		{"RUNNELIX_OIDC_ISSUER": "https://issuer.example"},
		{"RUNNELIX_AUTH_MODE": "oidc", "RUNNELIX_OIDC_ISSUER": "http://127.0.0.1:1234", "RUNNELIX_OIDC_CLIENT_ID": "fixture"},
		{"RUNNELIX_AUTH_MODE": "oidc", "RUNNELIX_OIDC_ISSUER": "http://localhost:1234", "RUNNELIX_OIDC_CLIENT_ID": "fixture", "RUNNELIX_DEV_OIDC_LOOPBACK": "1"},
		{"RUNNELIX_AUTH_MODE": "oidc", "RUNNELIX_OIDC_ISSUER": "https://user:password@issuer.example", "RUNNELIX_OIDC_CLIENT_ID": "fixture"},
		{"RUNNELIX_DEV_OIDC_LOOPBACK": "yes"},
	} {
		if _, err := identity.FromEnv(func(k string) string { return env[k] }); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	config, err := identity.FromEnv(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := identity.New(context.Background(), config)
	if err != nil || verifier != nil {
		t.Fatal("development mode changed", err)
	}
}
func TestOIDCVerifiesSignatureIssuerAudienceAndTime(t *testing.T) {
	f := issuer(t)
	v := f.verifier(t)
	ctx := context.Background()
	principal, err := v.Verify(ctx, f.token(t, nil))
	if err != nil || principal.Subject != "fixture-user" || principal.Issuer != f.server.URL {
		t.Fatal("valid identity rejected", err)
	}
	for name, change := range map[string]map[string]any{
		"issuer": {"iss": "https://other.example"}, "audience": {"aud": "other-client"}, "expired": {"exp": time.Now().Add(-time.Hour).Unix()}, "missing expiry": {"exp": nil}, "future": {"nbf": time.Now().Add(time.Hour).Unix()}, "future issuance": {"iat": time.Now().Add(time.Hour).Unix()}, "missing issued-at": {"iat": nil}, "null issued-at": {"iat": json.RawMessage("null")}, "malformed issued-at": {"iat": "bad"}, "missing subject": {"sub": ""}, "blank subject": {"sub": " "}, "wrong authorized party": {"azp": "other-client"}, "multiple audiences without party": {"aud": []string{"fixture-client", "other"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Verify(ctx, f.token(t, change)); !errors.Is(err, identity.ErrUnauthenticated) {
				t.Fatal("invalid identity accepted", err)
			}
		})
	}
	if _, err = v.Verify(ctx, f.token(t, map[string]any{"aud": []string{"fixture-client", "other"}, "azp": "fixture-client"})); err != nil {
		t.Fatal("valid authorized party rejected", err)
	}
	foreign := issuer(t)
	foreign.server = f.server
	for _, raw := range []string{foreign.token(t, nil), "eyJhbGciOiJub25lIn0.e30.", "not-a-token", strings.Repeat("x", 16385), "Bearer " + f.token(t, nil)} {
		if _, err = v.Verify(ctx, raw); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal("untrusted token accepted", err)
		}
	}
}
func TestOIDCAPIAuthenticatesWithoutGrantingWorkspaceAccess(t *testing.T) {
	f := issuer(t)
	handler := api.API{Store: &store.Store{}, Identity: f.verifier(t)}.Handler()
	token := f.token(t, nil)
	call := func(method, path string, auth []string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8787"+path, nil)
		for _, header := range auth {
			r.Header.Add("Authorization", header)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "/v1/identity", []string{"Bearer " + token})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "fixture-user") || strings.Contains(w.Body.String(), token) {
		t.Fatal("identity inspection failed", w.Code)
	}
	for _, headers := range [][]string{nil, {"wrong"}, {"Bearer invalid"}, {"Bearer " + token, "Bearer " + token}} {
		w = call("GET", "/v1/identity?access_token="+token, headers)
		if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" || strings.Contains(w.Body.String(), token) || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("missing or invalid auth accepted", w.Code)
		}
	}
	for _, path := range []string{"/v1/node-types", "/v1/workspaces/a/workflows", "/v1/workspaces/b/runs/known/events", "/v1/workspaces/a/runs/known/payloads/known", "/v1/workspaces/a/workflows/known/export"} {
		if got := call("GET", path, []string{"Bearer " + token}).Code; got != 403 {
			t.Fatal("identity granted data access", path, got)
		}
	}
	if got := call("POST", "/v1/workspaces/a/credentials", []string{"Bearer " + token}).Code; got != 403 {
		t.Fatal("identity granted mutation", got)
	}
	if got := call("GET", "/v1/workspaces/a/workflows", nil).Code; got != 401 {
		t.Fatal("workspace bypassed auth", got)
	}
}
func TestOIDCDiscoveryRejectsUnsafeEndpointsAndMismatches(t *testing.T) {
	for _, test := range []string{"issuer mismatch", "redirect", "oversized", "insecure keys"} {
		t.Run(test, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test == "redirect" {
					http.Redirect(w, r, "http://127.0.0.1:1/discovery", 302)
					return
				}
				if test == "oversized" {
					_, _ = w.Write([]byte(strings.Repeat(" ", 1024*1024+1)))
					return
				}
				iss := server.URL
				if test == "issuer mismatch" {
					iss = "https://other.example"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"issuer": iss, "jwks_uri": "http://non-loopback.invalid/keys"})
			}))
			defer server.Close()
			v, err := identity.New(context.Background(), identity.Config{Mode: "oidc", Issuer: server.URL, ClientID: "fixture-client", AllowLoopbackIssuer: true})
			if test != "insecure keys" {
				if err == nil {
					t.Fatal("unsafe discovery accepted")
				}
				return
			}
			if err != nil {
				return
			}
			f := issuer(t)
			f.server = server
			if _, err = v.Verify(context.Background(), f.token(t, nil)); !errors.Is(err, identity.ErrUnauthenticated) {
				t.Fatal("insecure key endpoint accepted", err)
			}
		})
	}
}
