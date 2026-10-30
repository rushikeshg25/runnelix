package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"net/url"
	"time"

	"golang.org/x/oauth2"
)

// Browser is a public OIDC client using authorization code and S256 PKCE.
// The supported deployment is the existing literal-loopback browser installation.
type Browser struct {
	verifier *Verifier
	config   oauth2.Config
	Origin   string
	Secure   bool
}

func NewBrowser(v *Verifier, origin, redirect string) (*Browser, error) {
	if redirect == "" {
		return nil, nil
	}
	u, err := url.Parse(origin)
	if err != nil || v == nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" || (u.Scheme != "http" && u.Scheme != "https") || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() || redirect != origin+"/v1/auth/callback" {
		return nil, errors.New("browser login requires OIDC and an exact literal-loopback origin/callback")
	}
	if !allowedURL(v.endpoint.AuthURL, v.allowLoopback) || !allowedURL(v.endpoint.TokenURL, v.allowLoopback) {
		return nil, errors.New("OIDC login endpoints require HTTPS or explicit loopback fixture mode")
	}
	endpoint := v.endpoint
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	return &Browser{verifier: v, config: oauth2.Config{ClientID: v.clientID, Endpoint: endpoint, RedirectURL: redirect, Scopes: []string{"openid"}}, Origin: origin, Secure: u.Scheme == "https"}, nil
}
func (b *Browser) Issuer() string   { return b.verifier.issuer }
func (b *Browser) ClientID() string { return b.verifier.clientID }
func (b *Browser) LoginURL(state, nonce, verifier string) string {
	return b.config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce))
}
func (b *Browser) Exchange(ctx context.Context, code, verifier, nonce string) (Principal, time.Time, error) {
	if code == "" || len(code) > 4096 {
		return Principal{}, time.Time{}, ErrUnauthenticated
	}
	token, err := b.config.Exchange(context.WithValue(ctx, oauth2.HTTPClient, b.verifier.client), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Principal{}, time.Time{}, ErrUnauthenticated
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return Principal{}, time.Time{}, ErrUnauthenticated
	}
	id, err := b.verifier.verify(ctx, raw)
	if err != nil || subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(nonce)) != 1 {
		return Principal{}, time.Time{}, ErrUnauthenticated
	}
	expires := time.Now().Add(time.Hour)
	if id.Expiry.Before(expires) {
		expires = id.Expiry
	}
	return Principal{Issuer: id.Issuer, Subject: id.Subject}, expires, nil
}
