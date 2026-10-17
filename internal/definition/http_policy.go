package definition

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// HTTPPolicy is trusted build-time metadata. Workflow authors cannot override it.
type HTTPPolicy struct {
	Method             string            `json:"method"`
	URL                string            `json:"url,omitempty"`
	URLConfig          string            `json:"urlConfig,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	CredentialRequired bool              `json:"credentialRequired,omitempty"`
}

var placeholder = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)

func (p HTTPPolicy) Target(config map[string]any) (string, error) {
	if p.Method != "GET" && p.Method != "POST" {
		return "", errors.New("invalid request policy")
	}
	if (p.URL == "") == (p.URLConfig == "") {
		return "", errors.New("select one request URL source")
	}
	target := p.URL
	if p.URLConfig != "" {
		target, _ = config[p.URLConfig].(string)
	} else {
		valid := true
		target = placeholder.ReplaceAllStringFunc(target, func(match string) string {
			value, ok := config[match[1:len(match)-1]].(string)
			if !ok || value == "" || value == "." || value == ".." {
				valid = false
			}
			return url.PathEscape(value)
		})
		if !valid {
			return "", errors.New("missing request path field")
		}
	}
	u, err := url.Parse(target)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("invalid request destination")
	}
	if p.URL != "" {
		// Templates may substitute path segments only, never authority or query values.
		template, err := url.Parse(p.URL)
		if err != nil || u.Scheme != "https" || u.Host != template.Host || u.RawQuery != template.RawQuery || strings.ContainsAny(template.Host+template.RawQuery, "{}") {
			return "", errors.New("invalid fixed request destination")
		}
	}
	return target, nil
}
