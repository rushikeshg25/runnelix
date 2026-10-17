package nodeservice

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEgressDestinations(t *testing.T) {
	e := Egress{Hosts: map[string]bool{"example.com": true, "127.0.0.1": true, "10.0.0.1": true, "169.254.169.254": true}}
	for _, target := range []string{"http://example.com", "https://example.com@evil.com", "https://10.0.0.1", "https://127.0.0.1", "https://169.254.169.254", "https://example.com/#fragment"} {
		if _, err := e.Validate(target); err == nil {
			t.Errorf("accepted %s", target)
		}
	}
	if _, err := e.Validate("https://example.com/resource"); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"::1", "::ffff:127.0.0.1", "100.64.0.1", "198.18.0.1", "224.1.2.3", "fd00::1"} {
		if public(net.ParseIP(address)) {
			t.Errorf("public %s", address)
		}
	}
}
func TestEgressResponsesAndCredentialEcho(t *testing.T) {
	secret := `sentinel-"\\-credential`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("missing credential")
		}
		switch r.URL.Path {
		case "/redirect":
			w.Header().Set("Location", "/ok")
			w.WriteHeader(302)
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", 8193)))
		case "/retry":
			w.WriteHeader(429)
		case "/bad":
			_, _ = w.Write([]byte("invalid"))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{secret: []any{secret, map[string]any{"value": "Bearer " + secret}}})
		}
	}))
	defer server.Close()
	e := Egress{Hosts: map[string]bool{"127.0.0.1": true}, AllowLoopback: true}
	for path, code := range map[string]string{"/redirect": "ProviderRejected", "/large": "InvalidOutput", "/retry": "ProviderTransient", "/bad": "InvalidOutput"} {
		_, err := e.Do(context.Background(), Request{URL: server.URL + path, Method: "GET"}, secret)
		if failure, ok := err.(*ProviderError); !ok || failure.Code != code {
			t.Errorf("%s: %v", path, err)
		}
	}
	result, err := e.Do(context.Background(), Request{URL: server.URL + "/ok", Method: "GET"}, secret)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "sentinel") || !strings.Contains(string(raw), "[redacted]") {
		t.Fatal("credential echo escaped redaction")
	}
}
func TestPublishedRequestBinding(t *testing.T) {
	config := map[string]any{"url": "https://example.com/one"}
	for _, request := range []Request{{URL: "https://example.com/two", Method: "GET"}, {URL: "https://example.com/one", Method: "POST"}} {
		if approvedRequest("http.get", config, request) {
			t.Fatal("request escaped published capability")
		}
	}
	if approvedRequest("mock.echo", config, Request{URL: "https://example.com/one", Method: "GET"}) {
		t.Fatal("echo acquired network capability")
	}
}
func TestAuthenticationRequiresBearer(t *testing.T) {
	token := strings.Repeat("t", 32)
	handler, err := (Service{Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	for _, auth := range []string{token, "Bearer wrong", ""} {
		r := httptest.NewRequest("POST", "/internal/node/input", strings.NewReader(`{}`))
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
}
