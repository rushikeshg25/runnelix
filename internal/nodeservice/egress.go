package nodeservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type ProviderError struct {
	Code      string
	Retryable bool
}

func (e *ProviderError) Error() string { return e.Code }

type Request struct {
	Headers map[string]string `json:"-"` // Only trusted catalog metadata can set headers.
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Body    any               `json:"body,omitempty"`
}
type HTTPResult struct {
	Status int `json:"status"`
	Body   any `json:"body"`
}
type Egress struct {
	Hosts         map[string]bool
	AllowLoopback bool
}

func public(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range []string{"100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(prefix).Contains(address) {
			return false
		}
	}
	return true
}
func (e Egress) Validate(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(raw) > 2048 {
		return nil, &ProviderError{"InvalidInput", false}
	}
	host := strings.ToLower(u.Hostname())
	if !e.Hosts[host] {
		return nil, &ProviderError{"EgressDenied", false}
	}
	ip := net.ParseIP(host)
	loopback := ip != nil && ip.IsLoopback() && e.AllowLoopback
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, &ProviderError{"EgressDenied", false}
	}
	if ip != nil && !public(ip) && !loopback {
		return nil, &ProviderError{"EgressDenied", false}
	}
	return u, nil
}
func (e Egress) Do(ctx context.Context, request Request, credential string) (HTTPResult, error) {
	u, err := e.Validate(request.URL)
	if err != nil {
		return HTTPResult{}, err
	}
	if request.Method != "GET" && request.Method != "POST" {
		return HTTPResult{}, &ProviderError{"InvalidInput", false}
	}
	var body []byte
	if request.Method == "POST" {
		body, err = json.Marshal(request.Body)
		if err != nil || len(body) > 8192 {
			return HTTPResult{}, &ProviderError{"InvalidInput", false}
		}
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second, DisableKeepAlives: true}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || !strings.EqualFold(host, u.Hostname()) {
			return nil, errors.New("destination rejected")
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("destination unavailable")
		}
		for _, ip := range ips {
			literal := net.ParseIP(u.Hostname())
			if !public(ip.IP) && !(e.AllowLoopback && literal != nil && literal.IsLoopback() && ip.IP.IsLoopback()) {
				return nil, errors.New("destination rejected")
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, request.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return HTTPResult{}, &ProviderError{"InvalidInput", false}
	}
	req.Header.Set("Accept", "application/json")
	for name, value := range request.Headers {
		if strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Cookie") || strings.EqualFold(name, "Host") {
			return HTTPResult{}, &ProviderError{"InvalidInput", false}
		}
		req.Header.Set(name, value)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	response, err := client.Do(req)
	if err != nil {
		return HTTPResult{}, &ProviderError{"ProviderNetwork", true}
	}
	defer response.Body.Close()
	if response.StatusCode == 429 || response.StatusCode >= 500 {
		return HTTPResult{}, &ProviderError{"ProviderTransient", true}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return HTTPResult{}, &ProviderError{"ProviderRejected", false}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(data) > 8192 {
		return HTTPResult{}, &ProviderError{"InvalidOutput", false}
	}
	var value any
	if len(data) == 0 {
		value = map[string]any{}
	} else if json.Unmarshal(data, &value) != nil {
		return HTTPResult{}, &ProviderError{"InvalidOutput", false}
	}
	return HTTPResult{Status: response.StatusCode, Body: redact(value, credential)}, nil
}

// Redact decoded strings and object keys before output can be persisted.
func redact(value any, secret string) any {
	if secret == "" {
		return value
	}
	switch v := value.(type) {
	case string:
		return strings.ReplaceAll(v, secret, "[redacted]")
	case []any:
		for i := range v {
			v[i] = redact(v[i], secret)
		}
		return v
	case map[string]any:
		result := map[string]any{}
		for key, item := range v {
			result[strings.ReplaceAll(key, secret, "[redacted]")] = redact(item, secret)
		}
		return result
	default:
		return value
	}
}
