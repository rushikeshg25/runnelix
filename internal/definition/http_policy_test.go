package definition

import "testing"

func TestHTTPPolicyPinsAuthorityAndEncodesPathFields(t *testing.T) {
	p := HTTPPolicy{Method: "POST", URL: "https://api.example.com/repos/{owner}/{repo}/issues"}
	target, err := p.Target(map[string]any{"owner": "team", "repo": "a/b?x=1"})
	if err != nil || target != "https://api.example.com/repos/team/a%2Fb%3Fx=1/issues" {
		t.Fatal(target, err)
	}
	for _, config := range []map[string]any{{}, {"owner": "team", "repo": ".."}} {
		if _, err = p.Target(config); err == nil {
			t.Fatal("invalid path field accepted")
		}
	}
	for _, raw := range []string{"https://{host}/issues", "https://example.com/?key={repo}", "http://example.com/issues", "https://user:secret@example.com/issues"} {
		if _, err = (HTTPPolicy{Method: "POST", URL: raw}).Target(map[string]any{"host": "evil.example", "repo": "x"}); err == nil {
			t.Fatal("invalid template accepted", raw)
		}
	}
}
