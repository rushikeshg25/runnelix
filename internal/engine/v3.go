// Version 3 adds explicit control semantics. Execute (V2) remains for old histories.
package engine

import (
	"errors"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/store"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"time"
)

const WorkflowNameV3 = "RunnelixExecuteV3"
const QueueV3 = "runnelix-engine-v3"

type ExecutionResult struct {
	State  string        `json:"state"`
	Events []store.Event `json:"events"`
}

func ExecuteV3(ctx workflow.Context, r store.StartRequest) (result ExecutionResult, err error) {
	if r.ExecutionMode != "execute" || workflow.GetInfo(ctx).WorkflowExecution.ID != store.RuntimeIDFor(r) {
		return ExecutionResult{}, temporal.NewNonRetryableApplicationError("Invalid execution identity", "InvalidInput", nil)
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Second, ScheduleToCloseTimeout: time.Minute, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 5, InitialInterval: time.Second}})
	var snapshot store.Snapshot
	if err = workflow.ExecuteActivity(ctx, "Bootstrap", r).Get(ctx, &snapshot); err != nil {
		return ExecutionResult{}, err
	}
	journal := []store.Event{}
	if err = workflow.SetQueryHandler(ctx, "ProjectionJournal", func(after int64) ([]store.Event, error) {
		result := []store.Event{}
		for _, e := range journal {
			if e.Sequence > after {
				result = append(result, e)
			}
			if len(result) == 200 {
				break
			}
		}
		return result, nil
	}); err != nil {
		return ExecutionResult{}, err
	}
	sequence := int64(0)
	projectionLock := workflow.NewMutex(ctx)
	uncertain := map[string]bool{}
	project := func(c workflow.Context, e store.Event) error {
		if err := projectionLock.Lock(c); err != nil {
			return err
		}
		defer projectionLock.Unlock()
		if e.NodeState == "uncertain" {
			uncertain[e.InvocationID] = true
		}
		if e.NodeState == "resolved" {
			delete(uncertain, e.InvocationID)
		}
		if len(uncertain) > 0 && (e.State == "running" || e.State == "waiting") {
			e.State = "needs_attention"
		}
		sequence++
		e.Sequence = sequence
		journal = append(journal, e)
		projectionCtx := workflow.WithActivityOptions(c, workflow.ActivityOptions{StartToCloseTimeout: 2 * time.Second, ScheduleToCloseTimeout: 3 * time.Second, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1}})
		if projectionErr := workflow.ExecuteActivity(projectionCtx, "Project", Projection{r, e}).Get(projectionCtx, nil); projectionErr != nil {
			workflow.GetLogger(c).Warn("Run projection delayed; journal retained for reconciliation")
		}
		return nil
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
		result = ExecutionResult{State: state, Events: journal}
		err = nil // Product failures are durable results; the journal remains available after closure.
	}()
	if snapshot.Plan.InterpreterVersion != 3 {
		return ExecutionResult{}, temporal.NewNonRetryableApplicationError("Unsupported interpreter version", "VersionMismatch", nil)
	}
	if snapshot.CancelRequested {
		return ExecutionResult{}, temporal.NewCanceledError("Cancellation requested before execution")
	}
	if err = project(ctx, store.Event{State: "running"}); err != nil {
		return ExecutionResult{}, err
	}
	outputs := map[definition.Endpoint]string{}
	for _, step := range snapshot.Plan.Steps {
		if step.Kind == "trigger" {
			outputs[definition.Endpoint{Node: step.ID, Port: "out"}] = snapshot.InputRef
			if err = project(ctx, store.Event{State: "running", NodeID: step.ID, NodeState: "succeeded", OutputRef: snapshot.InputRef, Port: "out"}); err != nil {
				return ExecutionResult{}, err
			}
			continue
		}
		inputRef := ""
		mergeInputs := map[string]string{}
		for _, edge := range snapshot.Plan.Edges {
			if edge.To.Node == step.ID {
				ref := outputs[edge.From]
				if ref != "" {
					inputRef = ref
					mergeInputs[edge.To.Port] = ref
				}
			}
		}
		if inputRef == "" {
			if err = project(ctx, store.Event{State: "running", NodeID: step.ID, NodeState: "skipped"}); err != nil {
				return ExecutionResult{}, err
			}
			continue
		}
		if err = project(ctx, store.Event{State: "running", NodeID: step.ID, NodeState: "executing"}); err != nil {
			return ExecutionResult{}, err
		}
		var ref, port string
		port = "out"
		request := StepRequest{r, step, inputRef}
		if step.Type == "control.wait" {
			seconds, _ := step.Config["seconds"].(float64)
			if seconds < 1 || seconds > 2592000 {
				return ExecutionResult{}, temporal.NewNonRetryableApplicationError("Invalid wait", "InvalidInput", nil)
			}
			if err = project(ctx, store.Event{State: "waiting", NodeID: step.ID, NodeState: "waiting"}); err != nil {
				return ExecutionResult{}, err
			}
			err = workflow.Sleep(ctx, time.Duration(seconds)*time.Second)
			ref = inputRef
		} else if step.Type == "control.merge" {
			err = workflow.ExecuteActivity(ctx, "Merge", MergeRequest{Run: r, Inputs: mergeInputs, Policy: step.Config["policy"].(string)}).Get(ctx, &ref)
		} else if step.Type == "control.map" {
			ref, err = mapV3(ctx, request, project)
		} else if step.Kind == "control" {
			var out ControlResult
			err = workflow.ExecuteActivity(ctx, "Control", request).Get(ctx, &out)
			ref, port = out.OutputRef, out.Port
		} else {
			ref, err = actionV3(ctx, request, "root", project)
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
					return ExecutionResult{}, persistErr
				}
			}
			return ExecutionResult{}, err
		}
		outputs[definition.Endpoint{Node: step.ID, Port: port}] = ref
		if err = project(ctx, store.Event{State: "running", NodeID: step.ID, NodeState: "succeeded", OutputRef: ref, Port: port}); err != nil {
			return ExecutionResult{}, err
		}
	}
	return ExecutionResult{State: "succeeded", Events: journal}, nil
}
