package mapping

import (
	"context"
	"testing"
)

func TestEvaluate(t *testing.T) {
	input := map[string]any{"priority": float64(5), "name": "demo"}
	v, err := Evaluate(context.Background(), `input.priority >= 4`, input)
	if err != nil || v != true {
		t.Fatal(v, err)
	}
	v, err = Evaluate(context.Background(), `{"title":input.name,"urgent":input.priority >= 4}`, input)
	if err != nil || v.(map[string]any)["title"] != "demo" {
		t.Fatal(v, err)
	}
	for _, expr := range []string{"input.missing", `http.get("https://example.com")`, "process.env", `input.priority +`, ""} {
		if _, err = Evaluate(context.Background(), expr, input); err == nil {
			t.Fatal("accepted invalid expression", expr)
		}
	}
}
func TestExpressionBudget(t *testing.T) {
	items := make([]any, 1000)
	for i := range items {
		items[i] = float64(i)
	}
	if _, err := Evaluate(context.Background(), "input.items.map(x, input.items.map(y, x + y))", map[string]any{"items": items}); err == nil {
		t.Fatal("unbounded expression accepted")
	}
}

func TestJSONNumberArithmetic(t *testing.T) {
	input := map[string]any{"priority": float64(5), "fraction": 2.5}
	value, err := Evaluate(context.Background(), `{"value": input.priority + 1, "fraction": input.fraction + 0.5}`, input)
	if err != nil {
		t.Fatal(err)
	}
	if value.(map[string]any)["value"] != float64(6) || value.(map[string]any)["fraction"] != float64(3) {
		t.Fatal(value)
	}
}
