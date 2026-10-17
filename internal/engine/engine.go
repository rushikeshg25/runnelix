// Package engine interprets pinned plans. Workflow code never performs I/O directly.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/mapping"
	"github.com/rushikeshg25/runnelix/internal/store"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const WorkflowName = "RunnelixExecuteV2"
const Queue = "runnelix-engine-v2"
const NodeActivity = "ExecuteInternalNodeV1"

func NodeQueue(digest string) string {
	return "runnelix-nodes-" + strings.TrimPrefix(digest, "sha256:")
}

type StepRequest struct {
	Run      store.StartRequest `json:"run"`
	Step     definition.Step    `json:"step"`
	InputRef string             `json:"inputRef"`
}
type ControlResult struct {
	OutputRef string `json:"outputRef"`
	Port      string `json:"port"`
}
type Completion struct {
	OutputRef string `json:"outputRef"`
	Attempt   int    `json:"attempt"`
}
type Selection struct {
	Envelope   definition.Envelope `json:"envelope"`
	Completion Completion          `json:"completion"`
}
type Projection struct {
	Run   store.StartRequest `json:"run"`
	Event store.Event        `json:"event"`
}

type Activities struct{ Store *store.Store }

func (a *Activities) Bootstrap(ctx context.Context, r store.StartRequest) (store.Snapshot, error) {
	if err := a.Store.RecordReceipt(ctx, r, activity.GetInfo(ctx).WorkflowExecution.ID); err != nil {
		return store.Snapshot{}, safe(err)
	}
	snap, err := a.Store.ExecutionSnapshot(ctx, r)
	return snap, safe(err)
}
func (a *Activities) Project(ctx context.Context, p Projection) error {
	return safe(a.Store.ApplyEvent(ctx, p.Run, p.Event))
}
func (a *Activities) Prepare(ctx context.Context, r StepRequest) (definition.Envelope, error) {
	run, err := a.Store.GetRun(ctx, r.Run.WorkspaceID, r.Run.RunID)
	if err != nil {
		return definition.Envelope{}, safe(err)
	}
	if run.CancelRequested {
		return definition.Envelope{}, temporal.NewCanceledError("Cancellation requested")
	}
	e, err := a.Store.PrepareInvocation(ctx, r.Run, r.Step, r.InputRef)
	return e, safe(err)
}
func (a *Activities) Select(ctx context.Context, s Selection) (string, error) {
	ref, err := a.Store.AcceptAttempt(ctx, s.Envelope, s.Completion.Attempt)
	if err != nil {
		return "", safe(err)
	}
	if ref != s.Completion.OutputRef {
		return "", temporal.NewNonRetryableApplicationError("Invalid completion reference", "InvalidResult", nil)
	}
	return ref, nil
}
func (a *Activities) Control(ctx context.Context, r StepRequest) (ControlResult, error) {
	raw, err := a.Store.Payload(ctx, r.Run.WorkspaceID, r.Run.RunID, r.InputRef)
	if err != nil {
		return ControlResult{}, safe(err)
	}
	var input any
	if json.Unmarshal(raw, &input) != nil {
		return ControlResult{}, safe(store.ErrInvalid)
	}
	expression, _ := r.Step.Config["expression"].(string)
	value, err := mapping.Evaluate(ctx, expression, input)
	if err != nil {
		return ControlResult{}, temporal.NewNonRetryableApplicationError("Mapping evaluation failed", "InvalidExpression", nil)
	}
	if r.Step.Type == "control.condition" {
		condition, ok := value.(bool)
		if !ok {
			return ControlResult{}, temporal.NewNonRetryableApplicationError("Condition must return a Boolean", "InvalidExpression", nil)
		}
		port := "false"
		if condition {
			port = "true"
		}
		return ControlResult{r.InputRef, port}, nil
	}
	if _, ok := value.(map[string]any); !ok {
		return ControlResult{}, temporal.NewNonRetryableApplicationError("Mapping must return an object", "InvalidExpression", nil)
	}
	ref, err := a.Store.PutPayload(ctx, r.Run.WorkspaceID, r.Run.RunID, value)
	return ControlResult{ref, "out"}, safe(err)
}
func safe(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrLimit) {
		return temporal.NewNonRetryableApplicationError("Execution limit reached", "LimitExceeded", nil)
	}
	if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrInvalid) || errors.Is(err, store.ErrNotFound) {
		return temporal.NewNonRetryableApplicationError("Execution contract rejected", "InvalidInput", nil)
	}
	return errors.New("Execution storage unavailable")
}

func Execute(ctx workflow.Context, r store.StartRequest) (result string, err error) {
	if r.ExecutionMode != "execute" || workflow.GetInfo(ctx).WorkflowExecution.ID != store.RuntimeIDFor(r) {
		return "", temporal.NewNonRetryableApplicationError("Invalid execution identity", "InvalidInput", nil)
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Second, ScheduleToCloseTimeout: time.Minute, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 5, InitialInterval: time.Second}})
	var snapshot store.Snapshot
	if err = workflow.ExecuteActivity(ctx, "Bootstrap", r).Get(ctx, &snapshot); err != nil {
		return "", err
	}
	sequence := int64(0)
	project := func(c workflow.Context, e store.Event) error {
		sequence++
		e.Sequence = sequence
		return workflow.ExecuteActivity(c, "Project", Projection{r, e}).Get(c, nil)
	}
	// Terminal projection uses a disconnected context so cancellation can be recorded durably.
	defer func() {
		state := "succeeded"
		code := ""
		if err != nil {
			state = "failed"
			code = "ExecutionFailed"
			if temporal.IsCanceledError(err) {
				state = "cancelled"
				code = ""
			}
		}
		disconnected, cancel := workflow.NewDisconnectedContext(ctx)
		defer cancel()
		if persistErr := project(disconnected, store.Event{State: state, ErrorCode: code}); persistErr != nil {
			err = persistErr
		}
		result = state
	}()
	if snapshot.Plan.InterpreterVersion != 2 {
		return "", temporal.NewNonRetryableApplicationError("Unsupported interpreter version", "VersionMismatch", nil)
	}
	if snapshot.CancelRequested {
		return "", temporal.NewCanceledError("Cancellation requested before execution")
	}
	if err = project(ctx, store.Event{State: "running"}); err != nil {
		return "", err
	}
	outputs := map[definition.Endpoint]string{}
	for _, step := range snapshot.Plan.Steps {
		if step.Kind == "trigger" {
			outputs[definition.Endpoint{Node: step.ID, Port: "out"}] = snapshot.InputRef
			if err = project(ctx, store.Event{State: "running", NodeID: step.ID, NodeState: "succeeded", OutputRef: snapshot.InputRef, Port: "out"}); err != nil {
				return "", err
			}
			continue
		}
		inputRef := ""
		for _, edge := range snapshot.Plan.Edges {
			if edge.To.Node == step.ID {
				inputRef = outputs[edge.From]
				break
			}
		}
		if inputRef == "" {
			if err = project(ctx, store.Event{State: "running", NodeID: step.ID, NodeState: "skipped"}); err != nil {
				return "", err
			}
			continue
		}
		if err = project(ctx, store.Event{State: "running", NodeID: step.ID, NodeState: "executing"}); err != nil {
			return "", err
		}
		var ref, port string
		port = "out"
		request := StepRequest{r, step, inputRef}
		if step.Kind == "control" {
			var out ControlResult
			err = workflow.ExecuteActivity(ctx, "Control", request).Get(ctx, &out)
			ref, port = out.OutputRef, out.Port
		} else {
			var envelope definition.Envelope
			if err = workflow.ExecuteActivity(ctx, "Prepare", request).Get(ctx, &envelope); err != nil {
				return "", err
			}
			attempts := int32(1)
			if step.RetrySafety == "pure" || step.RetrySafety == "read-only" || step.RetrySafety == "provider-idempotent" {
				attempts = 3
			}
			nodeCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: NodeQueue(step.ImplementationDigest), StartToCloseTimeout: 10 * time.Second, ScheduleToCloseTimeout: 45 * time.Second, HeartbeatTimeout: 2 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: attempts, InitialInterval: time.Second, MaximumInterval: 5 * time.Second}})
			var completion Completion
			err = workflow.ExecuteActivity(nodeCtx, NodeActivity, envelope).Get(nodeCtx, &completion)
			if err == nil {
				err = workflow.ExecuteActivity(ctx, "Select", Selection{envelope, completion}).Get(ctx, &ref)
			}
			if err != nil && !temporal.IsCanceledError(err) && attempts == 1 {
				var failure *temporal.ApplicationError
				knownPreSend := errors.As(err, &failure) && (failure.Type() == "InvalidInput" || failure.Type() == "VersionMismatch")
				if !knownPreSend {
					if projectErr := project(ctx, store.Event{State: "needs_attention", NodeID: step.ID, NodeState: "uncertain", ErrorCode: "UncertainOutcome"}); projectErr != nil {
						return "", projectErr
					}
					err = workflow.Await(ctx, func() bool { return false })
					return "", err
				}
			}
		}
		if err != nil {
			code := "ActionFailed"
			var failure *temporal.ApplicationError
			if errors.As(err, &failure) {
				switch failure.Type() {
				case "InvalidExpression", "InvalidInput", "ProviderRejected", "ProviderTransient", "ProviderNetwork":
					code = failure.Type()
				}
			}
			if !temporal.IsCanceledError(err) {
				if persistErr := project(ctx, store.Event{State: "running", NodeID: step.ID, NodeState: "failed", ErrorCode: code}); persistErr != nil {
					return "", persistErr
				}
			}
			return "", err
		}
		outputs[definition.Endpoint{Node: step.ID, Port: port}] = ref
		if err = project(ctx, store.Event{State: "running", NodeID: step.ID, NodeState: "succeeded", OutputRef: ref, Port: port}); err != nil {
			return "", err
		}
	}
	return "succeeded", nil
}
