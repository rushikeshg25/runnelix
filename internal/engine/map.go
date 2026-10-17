package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/mapping"
	"github.com/rushikeshg25/runnelix/internal/store"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"time"
)

type ScopedRequest struct {
	StepRequest
	Scope string `json:"scope"`
}
type AssembleRequest struct {
	Run  store.StartRequest `json:"run"`
	Refs []string           `json:"refs"`
}

func (a *Activities) PrepareScoped(ctx context.Context, r ScopedRequest) (definition.Envelope, error) {
	run, err := a.Store.GetRun(ctx, r.Run.WorkspaceID, r.Run.RunID)
	if err != nil {
		return definition.Envelope{}, safe(err)
	}
	if run.CancelRequested {
		return definition.Envelope{}, temporal.NewCanceledError("Cancellation requested")
	}
	e, err := a.Store.PrepareScopedInvocation(ctx, r.Run, r.Step, r.InputRef, r.Scope)
	return e, safe(err)
}
func (a *Activities) Expand(ctx context.Context, r StepRequest) ([]string, error) {
	raw, err := a.Store.Payload(ctx, r.Run.WorkspaceID, r.Run.RunID, r.InputRef)
	if err != nil {
		return nil, safe(err)
	}
	var input any
	if json.Unmarshal(raw, &input) != nil {
		return nil, safe(store.ErrInvalid)
	}
	value, err := mapping.Evaluate(ctx, r.Step.Config["collectionExpression"].(string), input)
	if err != nil {
		return nil, safe(store.ErrInvalid)
	}
	items, ok := value.([]any)
	limit, _ := r.Step.Config["maxItems"].(float64)
	if !ok || limit < 1 || limit > 1000 || len(items) > int(limit) {
		return nil, safe(store.ErrInvalid)
	}
	refs := make([]string, len(items))
	for i, item := range items {
		if _, ok := item.(map[string]any); !ok {
			return nil, safe(store.ErrInvalid)
		}
		if refs[i], err = a.Store.PutPayload(ctx, r.Run.WorkspaceID, r.Run.RunID, item); err != nil {
			return nil, safe(err)
		}
	}
	return refs, nil
}
func (a *Activities) Assemble(ctx context.Context, r AssembleRequest) (string, error) {
	items := []any{}
	for _, ref := range r.Refs {
		raw, err := a.Store.Payload(ctx, r.Run.WorkspaceID, r.Run.RunID, ref)
		if err != nil {
			return "", safe(err)
		}
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return "", safe(store.ErrInvalid)
		}
		items = append(items, value)
	}
	ref, err := a.Store.PutPayload(ctx, r.Run.WorkspaceID, r.Run.RunID, map[string]any{"items": items})
	return ref, safe(err)
}

type projector func(workflow.Context, store.Event) error

func actionV3(ctx workflow.Context, r StepRequest, scope string, project projector) (string, error) {
	var e definition.Envelope
	if err := workflow.ExecuteActivity(ctx, "PrepareScoped", ScopedRequest{r, scope}).Get(ctx, &e); err != nil {
		return "", err
	}
	attempts := int32(1)
	if r.Step.RetrySafety == "pure" || r.Step.RetrySafety == "read-only" || r.Step.RetrySafety == "provider-idempotent" {
		attempts = 3
	}
	nodeCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: NodeQueue(r.Step.ImplementationDigest), StartToCloseTimeout: 10 * time.Second, ScheduleToCloseTimeout: 45 * time.Second, HeartbeatTimeout: 2 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: attempts, InitialInterval: time.Second, MaximumInterval: 5 * time.Second}})
	var completion Completion
	err := workflow.ExecuteActivity(nodeCtx, NodeActivity, e).Get(nodeCtx, &completion)
	var ref string
	if err == nil {
		err = workflow.ExecuteActivity(ctx, "Select", Selection{e, completion}).Get(ctx, &ref)
	}
	if err != nil && !temporal.IsCanceledError(err) && attempts == 1 {
		var failure *temporal.ApplicationError
		known := errors.As(err, &failure) && (failure.Type() == "InvalidInput" || failure.Type() == "VersionMismatch" || failure.Type() == "EgressDenied" || failure.Type() == "CredentialUnavailable")
		if !known {
			if persistErr := project(ctx, store.Event{State: "needs_attention", NodeID: r.Step.ID, InvocationID: e.InvocationID, NodeState: "uncertain", ErrorCode: "UncertainOutcome"}); persistErr != nil {
				return "", persistErr
			}
			err = workflow.Await(ctx, func() bool { return false })
		}
	}
	return ref, err
}
func mapV3(ctx workflow.Context, r StepRequest, project projector) (string, error) {
	if r.Step.Body == nil {
		return "", temporal.NewNonRetryableApplicationError("Missing Map body", "InvalidInput", nil)
	}
	width, ok := r.Step.Config["concurrency"].(float64)
	if !ok || width < 1 || width > 10 {
		return "", temporal.NewNonRetryableApplicationError("Invalid Map concurrency", "InvalidInput", nil)
	}
	var refs []string
	if err := workflow.ExecuteActivity(ctx, "Expand", r).Get(ctx, &refs); err != nil {
		return "", err
	}
	results := make([]string, len(refs))
	for start := 0; start < len(refs); start += int(width) {
		end := min(start+int(width), len(refs))
		futures := []workflow.Future{}
		for index := start; index < end; index++ {
			future, settable := workflow.NewFuture(ctx)
			futures = append(futures, future)
			workflow.Go(ctx, func(c workflow.Context) {
				scope := fmt.Sprintf("item_%d", index)
				ref, err := actionV3(c, StepRequest{r.Run, *r.Step.Body, refs[index]}, scope, project)
				if err == nil {
					err = project(c, store.Event{State: "running", NodeID: r.Step.ID, InvocationID: "invocation_" + r.Step.ID + "_" + scope, NodeState: "succeeded", OutputRef: ref})
				}
				settable.Set(ref, err)
			})
		}
		var first error
		for offset, future := range futures {
			if err := future.Get(ctx, &results[start+offset]); err != nil && first == nil {
				first = err
			}
		}
		if first != nil {
			return "", first
		}
	}
	var result string
	err := workflow.ExecuteActivity(ctx, "Assemble", AssembleRequest{r.Run, results}).Get(ctx, &result)
	return result, err
}
