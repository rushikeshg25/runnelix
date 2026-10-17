package definition

import (
	"encoding/json"
	product "github.com/rushikeshg25/runnelix/contracts/product"
	"testing"
)

func TestMapCompilationPinsBodyAndRejectsNestedControls(t *testing.T) {
	raw, err := product.Files.ReadFile("catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog Catalog
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	graph := validGraph()
	graph.Nodes[1].Type = "control.map"
	graph.Nodes[1].Config = map[string]any{"collectionExpression": "input.items", "bodyType": "mock.echo", "bodyVersion": 1, "bodyConfig": map[string]any{}, "maxItems": 10, "concurrency": 2}
	encoded, _ := json.Marshal(graph)
	_, plan, _, err := Compile(encoded, catalog)
	if err != nil {
		t.Fatal(err)
	}
	body := plan.Steps[1].Body
	if body == nil || body.Type != "mock.echo" || body.Kind != "action" || body.ImplementationDigest == "" {
		t.Fatal("body not pinned")
	}
	for _, change := range []map[string]any{{"bodyType": "control.map"}, {"concurrency": 11}, {"maxItems": 1001}, {"bodyConfig": map[string]any{"unknown": true}}} {
		config := map[string]any{}
		for k, v := range graph.Nodes[1].Config {
			config[k] = v
		}
		for k, v := range change {
			config[k] = v
		}
		bad := graph
		bad.Nodes = append([]Node{}, graph.Nodes...)
		bad.Nodes[1].Config = config
		encoded, _ = json.Marshal(bad)
		if _, _, _, err = Compile(encoded, catalog); err == nil {
			t.Fatal("invalid map accepted", change)
		}
	}
}
