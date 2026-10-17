package definition

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedEnvelopeFixtures(t *testing.T) {
	b, err := os.ReadFile("../../contracts/product/activity.fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name  string
		Value json.RawMessage
		Valid bool
	}
	if err = json.Unmarshal(b, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			_, err := ParseEnvelope(f.Value)
			if (err == nil) != f.Valid {
				t.Fatalf("valid=%t error=%v", f.Valid, err)
			}
		})
	}
}
func TestBuiltinCatalogCompiles(t *testing.T) {
	c, err := BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(validGraph())
	_, _, _, err = Compile(data, c)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompiledPlanMatchesContract(t *testing.T) {
	c, err := BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(validGraph())
	_, plan, _, err := Compile(data, c)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err = json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if err = Schema("plan.schema.json", raw); err != nil {
		t.Fatal(err)
	}
}
