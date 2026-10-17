package engine

import (
	"context"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/store"
)

func (a *Activities) MarkUncertain(ctx context.Context, e definition.Envelope) error {
	return safe(a.Store.MarkUncertain(ctx, e))
}
func (a *Activities) Resolve(ctx context.Context, e definition.Envelope) (store.Resolution, error) {
	r, err := a.Store.AcceptResolution(ctx, e)
	return r, safe(err)
}
