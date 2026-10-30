package definition

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// PortableWorkflow contains authoring data only, never runtime state or credentials.
type PortableWorkflow struct {
	FormatVersion   int              `json:"formatVersion"`
	Name            string           `json:"name"`
	Graph           Graph            `json:"graph"`
	CredentialSlots []CredentialSlot `json:"credentialSlots"`
}
type CredentialSlot struct {
	Slot        string `json:"slot"`
	Kind        string `json:"kind"`
	Destination string `json:"destination"`
}
type credentialLocation struct {
	config      map[string]any
	destination string
}

func credentialLocations(g Graph, catalog Catalog) ([]credentialLocation, error) {
	locations := []credentialLocation{}
	for _, n := range g.Nodes {
		config := n.Config
		d, _ := catalog.Find(n.Type, n.Version)
		if n.Type == "control.map" {
			config = n.Config["bodyConfig"].(map[string]any)
			d, _ = catalog.Find(n.Config["bodyType"].(string), int(n.Config["bodyVersion"].(float64)))
		}
		if ref, _ := config["credentialRef"].(string); ref != "" {
			if d.HTTPPolicy == nil {
				return nil, fmt.Errorf("credential has no destination policy")
			}
			target, err := d.HTTPPolicy.Target(config)
			if err != nil {
				return nil, err
			}
			u, err := url.Parse(target)
			if err != nil {
				return nil, err
			}
			locations = append(locations, credentialLocation{config, strings.ToLower(u.Hostname())})
		}
	}
	return locations, nil
}
func ExportPortable(name string, data []byte, catalog Catalog) (PortableWorkflow, error) {
	g, _, _, err := Compile(data, catalog)
	if err != nil {
		return PortableWorkflow{}, err
	}
	locations, err := credentialLocations(g, catalog)
	if err != nil {
		return PortableWorkflow{}, err
	}
	result := PortableWorkflow{1, name, g, []CredentialSlot{}}
	for i, location := range locations {
		slot := fmt.Sprintf("slot_%d", i+1)
		location.config["credentialRef"] = slot
		result.CredentialSlots = append(result.CredentialSlots, CredentialSlot{slot, "bearer", location.destination})
	}
	return result, nil
}

// BindPortable validates all authoring contracts before replacing explicit slots.
func BindPortable(p PortableWorkflow, bindings map[string]string, catalog Catalog) ([]byte, error) {
	bad := func() ([]byte, error) {
		return nil, &Invalid{[]Issue{{"/bundle", "invalid_portable", "Invalid portable workflow or credential bindings"}}}
	}
	if p.FormatVersion != 1 || len(strings.TrimSpace(p.Name)) == 0 || len(p.Name) > 120 || p.CredentialSlots == nil {
		return bad()
	}
	data, err := json.Marshal(p.Graph)
	if err != nil {
		return bad()
	}
	g, _, _, err := Compile(data, catalog)
	if err != nil {
		return nil, err
	}
	locations, err := credentialLocations(g, catalog)
	if err != nil {
		return bad()
	}
	if len(locations) != len(p.CredentialSlots) || len(bindings) != len(locations) {
		return bad()
	}
	slots := map[string]CredentialSlot{}
	for _, slot := range p.CredentialSlots {
		if slot.Slot == "" || slots[slot.Slot].Slot != "" || slot.Kind != "bearer" || slot.Destination == "" {
			return bad()
		}
		slots[slot.Slot] = slot
	}
	for _, location := range locations {
		ref := location.config["credentialRef"].(string)
		slot, ok := slots[ref]
		credential := bindings[ref]
		if !ok || slot.Destination != location.destination || !strings.HasPrefix(credential, "credential_") || len(credential) > 128 {
			return bad()
		}
		location.config["credentialRef"] = credential
		delete(slots, ref)
	}
	if len(slots) != 0 {
		return bad()
	}
	return json.Marshal(g)
}
