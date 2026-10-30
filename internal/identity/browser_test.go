package identity_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rushikeshg25/runnelix/internal/api"
	"github.com/rushikeshg25/runnelix/internal/identity"
	"github.com/rushikeshg25/runnelix/internal/store"
	"github.com/rushikeshg25/runnelix/internal/testoidc"
)

func TestBrowserConfiguration(t *testing.T) {
	f, err := testoidc.New()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(f)
	defer server.Close()
	f.URL = server.URL
	v, err := identity.New(context.Background(), identity.Config{Mode: "oidc", Issuer: f.URL, ClientID: "fixture-client", AllowLoopbackIssuer: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"https://example.com", "http://localhost:5173", "http://127.0.0.1:5173/", "http://user@127.0.0.1:5173", "http://127.0.0.1:5173?x=1", "http://127.0.0.1:5173?"} {
		if _, err = identity.NewBrowser(v, origin, origin+"/v1/auth/callback"); err == nil {
			t.Fatalf("unsafe origin accepted: %s", origin)
		}
	}
	if _, err = identity.NewBrowser(nil, "http://127.0.0.1:5173", "http://127.0.0.1:5173/v1/auth/callback"); err == nil {
		t.Fatal("development login enabled")
	}
	if _, err = identity.NewBrowser(v, "http://127.0.0.1:5173", "http://127.0.0.1:5173/other"); err == nil {
		t.Fatal("redirect mismatch")
	}
	b, err := identity.NewBrowser(v, "https://127.0.0.1:5173", "https://127.0.0.1:5173/v1/auth/callback")
	if err != nil || !b.Secure {
		t.Fatal("secure origin rejected", err)
	}
}

func TestBrowserLoginSessionsAndCSRF(t *testing.T) {
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
	if err = s.CreateWorkspace(ctx, ws, "Browser fixture"); err != nil {
		t.Fatal(err)
	}
	f, err := testoidc.New()
	if err != nil {
		t.Fatal(err)
	}
	issuer := httptest.NewServer(f)
	defer issuer.Close()
	f.URL = issuer.URL
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	f.Redirect = origin + "/v1/auth/callback"
	verifier, err := identity.New(ctx, identity.Config{Mode: "oidc", Issuer: f.URL, ClientID: "fixture-client", AllowLoopbackIssuer: true})
	if err != nil {
		t.Fatal(err)
	}
	browser, err := identity.NewBrowser(verifier, origin, f.Redirect)
	if err != nil {
		t.Fatal(err)
	}
	handler := func() http.Handler {
		return api.API{Store: s, Identity: verifier, Browser: browser, BrowserOrigin: origin}.Handler()
	}
	server.Config.Handler = handler()
	server.Start()
	defer server.Close()
	owner := identity.Principal{Issuer: f.URL, Subject: "fixture-owner"}
	if err = s.ProvisionOwner(ctx, ws, owner); err != nil {
		t.Fatal(err)
	}
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	client := newClient()
	call := func(c *http.Client, method, path, csrf, requestOrigin string, status int) *http.Response {
		r, _ := http.NewRequest(method, origin+path, strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if requestOrigin != "" {
			r.Header.Set("Origin", requestOrigin)
		}
		res, e := c.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		if res.StatusCode != status {
			res.Body.Close()
			t.Fatalf("%s %s expected %d got %d", method, strings.Split(path, "?")[0], status, res.StatusCode)
		}
		return res
	}
	begin := func(c *http.Client) string {
		res := call(c, "GET", "/v1/auth/login", "", "", 302)
		for _, cookie := range res.Cookies() {
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
				t.Fatal("unsafe login cookie")
			}
		}
		auth, e := url.Parse(res.Header.Get("Location"))
		res.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		q := auth.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" {
			t.Fatal("missing PKCE/nonce")
		}
		q.Set("consent", "yes")
		auth.RawQuery = q.Encode()
		res, e = c.Get(auth.String())
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if res.StatusCode != 302 {
			t.Fatal("fixture authorization failed")
		}
		return strings.TrimPrefix(res.Header.Get("Location"), origin)
	}
	session := func(c *http.Client) (string, bool) {
		res := call(c, "GET", "/v1/auth/session", "", "", 200)
		defer res.Body.Close()
		var result struct {
			CSRF          string `json:"csrfToken"`
			Authenticated bool   `json:"authenticated"`
		}
		if json.NewDecoder(res.Body).Decode(&result) != nil {
			t.Fatal("invalid session response")
		}
		return result.CSRF, result.Authenticated
	}
	if _, ok := session(client); ok {
		t.Fatal("anonymous session authenticated")
	}
	call(client, "GET", "/v1/workspaces/"+ws+"/workflows", "", "", 401).Body.Close()
	// State must match the initiating cookie, and a second browser cannot use it.
	callback := begin(client)
	call(newClient(), "GET", callback, "", "", 303).Body.Close()
	wrong, _ := url.Parse(callback)
	q := wrong.Query()
	q.Set("state", "wrong")
	wrong.RawQuery = q.Encode()
	res := call(client, "GET", wrong.String(), "", "", 303)
	if !strings.HasSuffix(res.Header.Get("Location"), "?login=failed") {
		t.Fatal("wrong state accepted")
	}
	res.Body.Close()
	// PKCE, nonce, and the ten-minute challenge lifetime are checked independently.
	for _, mutation := range []string{"verifier", "nonce", "expires_at"} {
		callback = begin(client)
		cookieURL, _ := url.Parse(origin)
		var login string
		for _, c := range client.Jar.Cookies(cookieURL) {
			if c.Name == "runnelix_login" {
				login = c.Value
			}
		}
		query := `UPDATE login_challenges SET ` + mutation + `='incorrect' WHERE state_hash=$1`
		if mutation == "expires_at" {
			query = `UPDATE login_challenges SET expires_at=now()-interval '1 second' WHERE state_hash=$1`
		}
		if _, err = s.Pool.Exec(ctx, query, store.TokenHash(login)); err != nil {
			t.Fatal(err)
		}
		res = call(client, "GET", callback, "", "", 303)
		if !strings.HasSuffix(res.Header.Get("Location"), "?login=failed") {
			t.Fatalf("invalid %s accepted", mutation)
		}
		res.Body.Close()
	}
	callback = begin(client)
	res = call(client, "GET", callback, "", "", 303)
	if res.Header.Get("Location") != origin+"/" {
		t.Fatal("login failed")
	}
	var raw string
	for _, cookie := range res.Cookies() {
		if cookie.Name == "runnelix_session" {
			raw = cookie.Value
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge > 3600 {
				t.Fatal("unsafe session cookie")
			}
		}
	}
	res.Body.Close()
	if raw == "" {
		t.Fatal("session cookie missing")
	}
	csrf, ok := session(client)
	if !ok || csrf == "" || csrf == raw {
		t.Fatal("session unavailable")
	}
	var stored string
	if err = s.Pool.QueryRow(ctx, `SELECT token_hash FROM browser_sessions WHERE token_hash=$1`, store.TokenHash(raw)).Scan(&stored); err != nil || stored == raw {
		t.Fatal("session is not hashed")
	}
	call(client, "GET", callback, "", "", 303).Body.Close()
	csrf, ok = session(client)
	if !ok {
		t.Fatal("callback replay disrupted current session")
	}
	base := "/v1/workspaces/" + ws
	for _, headers := range [][2]string{{"", origin}, {csrf, ""}, {csrf, "http://127.0.0.1:1"}, {"bad", origin}} {
		call(client, "POST", base+"/workflows", headers[0], headers[1], 403).Body.Close()
	}
	call(client, "POST", base+"/workflows", csrf, origin, 400).Body.Close()
	call(client, "GET", base+"/workflows", "", "", 200).Body.Close()
	// Recreating the handler retains the database session; no memory cache is needed.
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", origin+base+"/workflows", nil)
	req.AddCookie(&http.Cookie{Name: "runnelix_session", Value: raw})
	handler().ServeHTTP(recorder, req)
	if recorder.Code != 200 {
		t.Fatal("session lost across handler restart")
	}
	// An existing cookie cannot be combined with a bearer identity.
	req = httptest.NewRequest("GET", origin+base+"/workflows", nil)
	req.AddCookie(&http.Cookie{Name: "runnelix_session", Value: raw})
	req.Header.Set("Authorization", "Bearer invalid")
	recorder = httptest.NewRecorder()
	handler().ServeHTTP(recorder, req)
	if recorder.Code != 401 {
		t.Fatal("mixed authentication accepted")
	}
	// A real cookie-authenticated event stream ends after logout.
	graph := []byte(`{"formatVersion":1,"nodes":[{"id":"start","type":"trigger.manual","version":1,"config":{}},{"id":"echo","type":"mock.echo","version":1,"config":{}}],"edges":[{"from":{"node":"start","port":"out"},"to":{"node":"echo","port":"in"}}]}`)
	workflow, e := s.CreateWorkflow(ctx, ws, "Session stream", graph)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Publish(ctx, ws, workflow.ID, 1); e != nil {
		t.Fatal(e)
	}
	run, e := s.AdmitExecution(ctx, ws, workflow.ID, "session-run", []byte(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	req, _ = http.NewRequest("GET", origin+base+"/runs/"+run.ID+"/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	stream, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer stream.Body.Close()
	if stream.StatusCode != 200 {
		t.Fatal("session SSE rejected")
	}
	reader := bufio.NewReader(stream.Body)
	if _, e = reader.ReadString('\n'); e != nil {
		t.Fatal(e)
	}
	call(client, "POST", "/v1/auth/logout", csrf, origin, 200).Body.Close()
	if _, e = io.ReadAll(reader); e != nil {
		t.Fatal("logout did not close SSE")
	}
	req = httptest.NewRequest("GET", origin+base+"/workflows", nil)
	req.AddCookie(&http.Cookie{Name: "runnelix_session", Value: raw})
	recorder = httptest.NewRecorder()
	handler().ServeHTTP(recorder, req)
	if recorder.Code != 401 {
		t.Fatal("logged-out cookie replayed")
	}
	callback = begin(client)
	call(client, "GET", callback, "", "", 303).Body.Close()
	if _, err = s.Pool.Exec(ctx, `UPDATE browser_sessions SET expires_at=now()-interval '1 second' WHERE issuer=$1`, f.URL); err != nil {
		t.Fatal(err)
	}
	if _, ok = session(client); ok {
		t.Fatal("expired session accepted")
	}
}

func TestDisabledBrowserLoginReportsStatusWithStaleCookie(t *testing.T) {
	f := issuer(t)
	handler := api.API{Store: &store.Store{}, Identity: f.verifier(t)}.Handler()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8787/v1/auth/session", nil)
	r.AddCookie(&http.Cookie{Name: "runnelix_session", Value: strings.Repeat("a", 43)})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	var status struct {
		Authenticated bool   `json:"authenticated"`
		LoginEnabled  bool   `json:"loginEnabled"`
		Mode          string `json:"mode"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status.Authenticated || status.LoginEnabled || status.Mode != "oidc" {
		t.Fatal("stale cookie hid disabled login status")
	}
}
