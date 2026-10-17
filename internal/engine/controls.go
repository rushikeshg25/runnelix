package engine

import (
	"context"
	"encoding/json"
	"github.com/rushikeshg25/runnelix/internal/store"
	"sort"
)

type MergeRequest struct {
	Run    store.StartRequest `json:"run"`
	Inputs map[string]string  `json:"inputs"`
	Policy string             `json:"policy"`
}

func (a *Activities) Merge(ctx context.Context, r MergeRequest) (string, error) {
	values := map[string]any{}
	keys := []string{}
	for key := range r.Inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := []any{}
	for _, key := range keys {
		raw, err := a.Store.Payload(ctx, r.Run.WorkspaceID, r.Run.RunID, r.Inputs[key])
		if err != nil {
			return "", safe(err)
		}
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return "", safe(store.ErrInvalid)
		}
		values[key] = value
		ordered = append(ordered, value)
	}
	var result any = values
	switch r.Policy {
	case "named":
	case "ordered":
		result = map[string]any{"items": ordered}
	default:
		return "", safe(store.ErrInvalid)
	}
	ref, err := a.Store.PutPayload(ctx, r.Run.WorkspaceID, r.Run.RunID, result)
	return ref, safe(err)
}
