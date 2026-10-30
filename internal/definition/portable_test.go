package definition

import (
	"encoding/json"
	"strings"
	"testing"
)

func portableFixture(t *testing.T) (PortableWorkflow, Catalog) {
	t.Helper()
	c, err := BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"formatVersion":1,"nodes":[{"id":"start","type":"trigger.manual","version":1,"config":{}},{"id":"send","type":"control.map","version":1,"config":{"collectionExpression":"input.items","bodyType":"slack.postMessage","bodyVersion":1,"bodyConfig":{"channel":"C123","credentialRef":"credential_private"},"maxItems":10,"concurrency":1}}],"edges":[{"from":{"node":"start","port":"out"},"to":{"node":"send","port":"in"}}]}`)
	p, err := ExportPortable("Fixture", data, c)
	if err != nil {
		t.Fatal(err)
	}
	return p, c
}
func TestPortableMapRoundTrip(t *testing.T) {
	p, c := portableFixture(t)
	data, _ := json.Marshal(p)
	if strings.Contains(string(data), "credential_private") || len(p.CredentialSlots) != 1 || p.CredentialSlots[0].Destination != "slack.com" {
		t.Fatal("export leaked reference or lost Map slot")
	}
	bound, err := BindPortable(p, map[string]string{"slot_1": "credential_destination"}, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = Compile(bound, c); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bound), "credential_destination") {
		t.Fatal("binding absent")
	}
}
func TestPortableRejectsInvalidBindingsAndVersions(t *testing.T) {
	for _, change := range []func(*PortableWorkflow, map[string]string){
		func(p *PortableWorkflow, b map[string]string) { p.FormatVersion = 2 },
		func(p *PortableWorkflow, b map[string]string) { p.Graph.Nodes[1].Version = 999 },
		func(p *PortableWorkflow, b map[string]string) { p.CredentialSlots[0].Destination = "evil.example" },
		func(p *PortableWorkflow, b map[string]string) {
			p.CredentialSlots = append(p.CredentialSlots, p.CredentialSlots[0])
		},
		func(p *PortableWorkflow, b map[string]string) { delete(b, "slot_1") },
		func(p *PortableWorkflow, b map[string]string) { b["extra"] = "credential_other" },
		func(p *PortableWorkflow, b map[string]string) {
			p.Graph.Nodes[1].Config["bodyConfig"].(map[string]any)["credentialRef"] = "credential_source"
		},
	} {
		p, c := portableFixture(t)
		bindings := map[string]string{"slot_1": "credential_destination"}
		change(&p, bindings)
		if _, err := BindPortable(p, bindings, c); err == nil {
			t.Fatal("invalid portable import accepted")
		}
	}
}
