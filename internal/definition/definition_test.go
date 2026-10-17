package definition

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func testCatalog() Catalog {
	empty := map[string]any{"type": "object", "additionalProperties": false}
	return Catalog{{Type: "trigger.manual", Version: 1, Kind: "trigger", Outputs: []string{"out"}, ImplementationDigest: "test-trigger", ConfigSchema: empty}, {Type: "mock.echo", Version: 1, Kind: "action", Inputs: []string{"in"}, Outputs: []string{"out"}, ImplementationDigest: "test-echo", ConfigSchema: empty}}
}
func validGraph() Graph {
	return Graph{1, []Node{{"start", "trigger.manual", 1, map[string]any{}}, {"echo", "mock.echo", 1, map[string]any{}}}, []Edge{{Endpoint{"start", "out"}, Endpoint{"echo", "in"}}}}
}
func TestCompileFailures(t *testing.T) {
	tests := []struct {
		name, code string
		change     func(*Graph)
	}{
		{"duplicate", "duplicate_id", func(g *Graph) { g.Nodes = append(g.Nodes, g.Nodes[0]) }},
		{"unknown node", "unknown_type", func(g *Graph) { g.Nodes[1].Type = "external.code" }},
		{"unavailable version", "unknown_type", func(g *Graph) { g.Nodes[1].Version = 2 }},
		{"invalid config", "invalid_config", func(g *Graph) { g.Nodes[1].Config["secret"] = "sentinel" }},
		{"port", "unknown_port", func(g *Graph) { g.Edges[0].From.Port = "wrong" }},
		{"dangling", "dangling_edge", func(g *Graph) { g.Edges[0].From.Node = "absent" }},
		{"missing input", "missing_input", func(g *Graph) { g.Edges = []Edge{} }},
		{"multiple input", "multiple_inputs", func(g *Graph) { g.Edges = append(g.Edges, g.Edges[0]) }},
		{"two triggers", "trigger_count", func(g *Graph) { g.Nodes = append(g.Nodes, Node{"second", "trigger.manual", 1, map[string]any{}}) }},
		{"cycle", "cycle", func(g *Graph) {
			g.Nodes = append(g.Nodes, Node{"other", "mock.echo", 1, map[string]any{}})
			g.Edges = []Edge{{Endpoint{"other", "out"}, Endpoint{"echo", "in"}}, {Endpoint{"echo", "out"}, Endpoint{"other", "in"}}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := validGraph()
			tt.change(&g)
			b, _ := json.Marshal(g)
			_, _, _, err := Compile(b, testCatalog())
			var invalid *Invalid
			if !errors.As(err, &invalid) || invalid.Issues[0].Code != tt.code {
				t.Fatalf("wanted %s; got %v", tt.code, err)
			}
		})
	}
}
func TestDeterministicPlan(t *testing.T) {
	g := validGraph()
	a, _ := json.Marshal(g)
	_, p, first, err := Compile(a, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	g.Nodes[0], g.Nodes[1] = g.Nodes[1], g.Nodes[0]
	b, _ := json.Marshal(g)
	_, _, second, err := Compile(b, testCatalog())
	if err != nil || first != second {
		t.Fatal("plan depends on editor node ordering")
	}
	if p.Steps[0].ID != "start" || p.Steps[1].ImplementationDigest != "test-echo" {
		t.Fatal("plan is not pinned/topological")
	}
}
func TestStrictJSON(t *testing.T) {
	for _, s := range []string{`{"a":1,"a":2}`, `{} {}`, `{"x":9007199254740992}`, strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34), strings.Repeat(" ", MaxBytes+1) + "{}"} {
		var v any
		if Decode([]byte(s), &v) == nil {
			t.Fatal("accepted invalid or unbounded JSON")
		}
	}
}
