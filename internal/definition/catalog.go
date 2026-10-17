package definition

import (
	"encoding/json"
	"fmt"
	product "github.com/rushikeshg25/runnelix/contracts/product"
)

func BuiltinCatalog() (Catalog, error) {
	data, err := product.Files.ReadFile("catalog.json")
	if err != nil {
		return nil, err
	}
	var catalog Catalog
	if err = json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, d := range catalog {
		key := fmt.Sprintf("%s@%d", d.Type, d.Version)
		if seen[key] || len(d.ImplementationDigest) != 71 {
			return nil, fmt.Errorf("invalid catalog")
		}
		seen[key] = true
	}
	return catalog, nil
}

type Envelope struct {
	ContractVersion      int    `json:"contractVersion"`
	WorkspaceID          string `json:"workspaceId"`
	RunID                string `json:"runId"`
	NodeID               string `json:"nodeId"`
	InvocationID         string `json:"invocationId"`
	NodeType             string `json:"nodeType"`
	NodeVersion          int    `json:"nodeVersion"`
	ImplementationDigest string `json:"implementationDigest"`
	InputRef             string `json:"inputRef"`
	ConfigRef            string `json:"configRef"`
	LogicalActionKey     string `json:"logicalActionKey"`
}

func ParseEnvelope(data []byte) (Envelope, error) {
	var e Envelope
	var raw any
	if err := Decode(data, &raw); err != nil {
		return e, err
	}
	if err := Schema("activity.schema.json", raw); err != nil {
		return e, fmt.Errorf("invalid activity envelope")
	}
	err := Decode(data, &e)
	return e, err
}
