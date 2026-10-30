// Package testoidc provides an explicitly fake, loopback-only OIDC issuer for
// integration tests and local browser verification. Never use it for real users.
package testoidc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type code struct{ Challenge, Nonce, Redirect string }
type Issuer struct {
	URL      string
	Redirect string
	key      *rsa.PrivateKey
	mu       sync.Mutex
	codes    map[string]code
}

func New() (*Issuer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &Issuer{key: key, codes: map[string]code{}}, nil
}
func (f *Issuer) token(nonce string) string {
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "fixture"})
	payload, _ := json.Marshal(map[string]any{"iss": f.URL, "sub": "fixture-owner", "aud": "fixture-client", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": nonce})
	data := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(data))
	signature, _ := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	return data + "." + base64.RawURLEncoding.EncodeToString(signature)
}
func (f *Issuer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": f.URL, "jwks_uri": f.URL + "/keys", "authorization_endpoint": f.URL + "/authorize", "token_endpoint": f.URL + "/token", "response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
	case "/keys":
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "fixture", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(f.key.N.Bytes()), "e": "AQAB"}}})
	case "/authorize":
		q := r.URL.Query()
		if q.Get("client_id") != "fixture-client" || q.Get("redirect_uri") != f.Redirect || q.Get("response_type") != "code" || q.Get("scope") != "openid" || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 || q.Get("state") == "" || q.Get("nonce") == "" {
			http.Error(w, "invalid fixture request", 400)
			return
		}
		if q.Get("consent") != "yes" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			page := template.Must(template.New("consent").Parse(`<!doctype html><html><meta name="viewport" content="width=device-width"><title>Local test identity</title><body><h1>Local test identity</h1><p>This provider is only for fake-data verification.</p><form method="get">{{range $key,$values:=.}}{{range $values}}<input type="hidden" name="{{$key}}" value="{{.}}">{{end}}{{end}}<button name="consent" value="yes">Continue as fixture owner</button></form></body></html>`))
			_ = page.Execute(w, q)
			return
		}
		raw := make([]byte, 32)
		_, _ = rand.Read(raw)
		key := base64.RawURLEncoding.EncodeToString(raw)
		f.mu.Lock()
		f.codes[key] = code{q.Get("code_challenge"), q.Get("nonce"), q.Get("redirect_uri")}
		f.mu.Unlock()
		target, _ := url.Parse(f.Redirect)
		target.RawQuery = url.Values{"code": {key}, "state": {q.Get("state")}}.Encode()
		http.Redirect(w, r, target.String(), 302)
	case "/token":
		if r.Method != "POST" {
			http.Error(w, "method", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		if r.ParseForm() != nil {
			http.Error(w, "invalid", 400)
			return
		}
		f.mu.Lock()
		c, ok := f.codes[r.Form.Get("code")]
		delete(f.codes, r.Form.Get("code"))
		f.mu.Unlock()
		digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || r.Form.Get("client_id") != "fixture-client" || r.Form.Get("grant_type") != "authorization_code" || c.Redirect != r.Form.Get("redirect_uri") || c.Challenge != base64.RawURLEncoding.EncodeToString(digest[:]) {
			http.Error(w, `{"error":"invalid_grant"}`, 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-unused", "token_type": "Bearer", "expires_in": 3600, "id_token": f.token(c.Nonce)})
	default:
		http.NotFound(w, r)
	}
}
