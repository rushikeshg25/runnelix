package spike

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSharedFixtures(t *testing.T) {
	data, err := os.ReadFile("../../contracts/spike/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Base  map[string]json.RawMessage `json:"base"`
		Cases []struct {
			Name  string
			Patch map[string]json.RawMessage
			Valid bool
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixtures.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			input := make(map[string]json.RawMessage)
			for k, v := range fixtures.Base {
				input[k] = v
			}
			for k, v := range tc.Patch {
				input[k] = v
			}
			b, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			_, err = DecodeRequest(b)
			if (err == nil) != tc.Valid {
				t.Fatalf("valid=%v error=%v", tc.Valid, err)
			}
		})
	}
}

func TestRejectMissingTrailingAndOversized(t *testing.T) {
	for _, s := range []string{`{}`, `null`, `{} {}`, strings.Repeat(" ", 8193)} {
		if _, err := DecodeRequest([]byte(s)); err == nil {
			t.Fatalf("accepted invalid input")
		}
	}
	v := map[string]any{}
	for range 18 {
		v = map[string]any{"nested": v}
	}
	r := Request{1, "deep", "v1", "safe", "none", 0, v}
	if r.Validate() == nil {
		t.Fatal("accepted excessive depth")
	}
}
