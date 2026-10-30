package api

import (
	"github.com/rushikeshg25/runnelix/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStrictRequestBoundaries(t *testing.T) {
	for _, raw := range []string{`{"name":"a","name":"b"}`, `{"name":"a","unexpected":true}`, `{} {}`, strings.Repeat(" ", 300000) + `{}`} {
		req := httptest.NewRequest("POST", "/", strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		var value struct {
			Name string `json:"name"`
		}
		if body(httptest.NewRecorder(), req, &value) == nil {
			t.Fatal("accepted invalid body")
		}
	}
}
func TestRevisionAndOrigin(t *testing.T) {
	for _, value := range []string{"", "1", "*", `W/"1"`, `"0"`, `"9007199254740992"`} {
		r := httptest.NewRequest("PUT", "/", nil)
		r.Header.Set("If-Match", value)
		if _, err := revision(r); err == nil {
			t.Fatal("accepted invalid revision", value)
		}
	}
	r := httptest.NewRequest("GET", "/v1/node-types", nil)
	r.Header.Set("Origin", "https://external.example")
	w := httptest.NewRecorder()
	API{Store: &store.Store{}}.Handler().ServeHTTP(w, r)
	if w.Code != 403 || w.Header().Get("X-Request-ID") == "" {
		t.Fatal("origin boundary or request ID missing")
	}
}

func TestRetentionExpiryResponse(t *testing.T) {
	w := httptest.NewRecorder()
	fail(w, httptest.NewRequest("GET", "/", nil), store.ErrExpired)
	if w.Code != 410 || !strings.Contains(w.Body.String(), `"code":"data_expired"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
