// Package identity verifies OIDC identity tokens; it does not grant workspace access.
package identity

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var ErrUnauthenticated = errors.New("identity could not be verified")

type Principal struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}
type Config struct {
	Mode                string
	Issuer              string
	ClientID            string
	AllowLoopbackIssuer bool
}

func FromEnv(getenv func(string) string) (Config, error) {
	c := Config{Mode: getenv("RUNNELIX_AUTH_MODE"), Issuer: getenv("RUNNELIX_OIDC_ISSUER"), ClientID: getenv("RUNNELIX_OIDC_CLIENT_ID")}
	if c.Mode == "" {
		c.Mode = "development"
	}
	switch getenv("RUNNELIX_DEV_OIDC_LOOPBACK") {
	case "", "0":
	case "1":
		c.AllowLoopbackIssuer = true
	default:
		return Config{}, errors.New("invalid OIDC loopback setting")
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	switch c.Mode {
	case "development":
		if c.Issuer != "" || c.ClientID != "" || c.AllowLoopbackIssuer {
			return errors.New("OIDC settings require explicit oidc authentication mode")
		}
	case "oidc":
		if strings.TrimSpace(c.ClientID) == "" || len(c.ClientID) > 256 || (!allowedURL(c.Issuer, c.AllowLoopbackIssuer) || strings.Contains(c.Issuer, "?")) {
			return errors.New("oidc mode requires a valid issuer and client ID")
		}
	default:
		return errors.New("unknown authentication mode")
	}
	return nil
}
func allowedURL(raw string, loopback bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return loopback && u.Scheme == "http" && ip != nil && ip.IsLoopback()
}

type boundedTransport struct{ loopback bool }

func (t boundedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !allowedURL(r.URL.String(), t.loopback) {
		return nil, errors.New("OIDC endpoint requires HTTPS or explicit literal loopback fixture mode")
	}
	response, err := http.DefaultTransport.RoundTrip(r)
	if err == nil {
		response.Body = http.MaxBytesReader(nil, response.Body, 1024*1024)
	}
	return response, err
}

type Verifier struct {
	verifier      *oidc.IDTokenVerifier
	clientID      string
	issuer        string
	endpoint      oauth2.Endpoint
	client        *http.Client
	allowLoopback bool
}

func New(ctx context.Context, c Config) (*Verifier, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Mode == "development" {
		return nil, nil
	}
	client := &http.Client{Transport: boundedTransport{c.AllowLoopbackIssuer}, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), c.Issuer)
	if err != nil {
		return nil, errors.New("OIDC discovery failed")
	}
	return &Verifier{verifier: provider.Verifier(&oidc.Config{ClientID: c.ClientID, SupportedSigningAlgs: []string{oidc.RS256}}), clientID: c.ClientID, issuer: c.Issuer, endpoint: provider.Endpoint(), client: client, allowLoopback: c.AllowLoopbackIssuer}, nil
}
func (v *Verifier) Verify(ctx context.Context, raw string) (Principal, error) {
	token, err := v.verify(ctx, raw)
	if err != nil {
		return Principal{}, err
	}
	return Principal{token.Issuer, token.Subject}, nil
}
func (v *Verifier) verify(ctx context.Context, raw string) (*oidc.IDToken, error) {
	if v == nil || len(raw) == 0 || len(raw) > 16384 || strings.ContainsAny(raw, " \t\r\n") {
		return nil, ErrUnauthenticated
	}
	token, err := v.verifier.Verify(ctx, raw)
	if err != nil || strings.TrimSpace(token.Subject) == "" || len(token.Subject) > 255 {
		return nil, ErrUnauthenticated
	}
	var claims struct {
		IssuedAt        *int64 `json:"iat"`
		AuthorizedParty string `json:"azp"`
		NotBefore       *int64 `json:"nbf"`
	}
	if token.Claims(&claims) != nil || claims.IssuedAt == nil {
		return nil, ErrUnauthenticated
	}
	now := time.Now()
	if (len(token.Audience) > 1 && claims.AuthorizedParty != v.clientID) || (claims.AuthorizedParty != "" && claims.AuthorizedParty != v.clientID) || (claims.NotBefore != nil && *claims.NotBefore > now.Unix()) || token.IssuedAt.After(now.Add(30*time.Second)) {
		return nil, ErrUnauthenticated
	}
	return token, nil
}
