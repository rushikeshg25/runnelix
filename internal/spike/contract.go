package spike

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
)

// Request is deliberately limited to the Phase 0 runtime experiment.
type Request struct {
	ContractVersion int            `json:"contractVersion"`
	RunID           string         `json:"runId"`
	NodeVersion     string         `json:"nodeVersion"`
	Mode            string         `json:"mode"`
	Fault           string         `json:"fault"`
	WaitMS          int            `json:"waitMs"`
	Value           map[string]any `json:"value"`
}

type Result struct {
	Status        string         `json:"status"`
	ActionID      string         `json:"actionId"`
	WorkerVersion string         `json:"workerVersion"`
	Attempts      int            `json:"attempts"`
	Value         map[string]any `json:"value"`
}

var identity = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

func DecodeRequest(data []byte) (Request, error) {
	var r Request
	if len(data) > 8192 {
		return r, fmt.Errorf("request exceeds 8192 bytes")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return r, err
	}
	for _, name := range []string{"contractVersion", "runId", "nodeVersion", "mode", "fault", "waitMs", "value"} {
		if fields[name] == nil || bytes.Equal(fields[name], []byte("null")) {
			return r, fmt.Errorf("missing or null %s", name)
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err := d.Decode(&r); err != nil {
		return r, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return r, fmt.Errorf("trailing JSON")
	}
	return r, r.Validate()
}

func (r Request) Validate() error {
	if r.ContractVersion != 1 || !identity.MatchString(r.RunID) {
		return fmt.Errorf("invalid contract version or run ID")
	}
	if r.NodeVersion != "v1" && r.NodeVersion != "v2" {
		return fmt.Errorf("unsupported node version")
	}
	if r.Mode != "safe" && r.Mode != "unsafe" {
		return fmt.Errorf("unsupported action mode")
	}
	if r.Fault != "none" && r.Fault != "transient-once" && r.Fault != "disconnect" && r.Fault != "hold" {
		return fmt.Errorf("unsupported fault")
	}
	if r.WaitMS < 0 || r.WaitMS > 60000 || r.Value == nil {
		return fmt.Errorf("invalid wait or payload")
	}
	b, err := json.Marshal(r)
	if err != nil || len(b) > 8192 {
		return fmt.Errorf("invalid or oversized request")
	}
	return validateJSON(r.Value, 0)
}

func validateJSON(value any, depth int) error {
	if depth > 16 {
		return fmt.Errorf("payload nesting exceeds 16")
	}
	switch v := value.(type) {
	case nil, string, bool:
		return nil
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return err
		}
		return validateJSON(n, depth)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 9007199254740991 {
			return fmt.Errorf("number outside interoperable range; encode large integers as strings")
		}
	case map[string]any:
		for _, x := range v {
			if err := validateJSON(x, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, x := range v {
			if err := validateJSON(x, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported JSON value")
	}
	return nil
}

func NodeQueue(version string) string     { return "runnelix-spike-nodes-" + version }
func WorkflowQueue(version string) string { return "runnelix-spike-go-" + version }
