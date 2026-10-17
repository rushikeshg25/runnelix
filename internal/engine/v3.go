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

func ExecuteV3(ctx workflow.Context, r store.StartRequest) (result string, err error) {
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
	if snapshot.Plan.InterpreterVersion != 3 {
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
		if step.Type == "control.wait" {
			seconds, _ := step.Config["seconds"].(float64)
			if seconds < 1 || seconds > 2592000 {
				return "", temporal.NewNonRetryableApplicationError("Invalid wait", "InvalidInput", nil)
			}
			if err = project(ctx, store.Event{State: "waiting", NodeID: step.ID, NodeState: "waiting"}); err != nil {
				return "", err
			}
			err = workflow.Sleep(ctx, time.Duration(seconds)*time.Second)
			ref = inputRef
		} else if step.Type == "control.merge" {
			err = workflow.ExecuteActivity(ctx, "Merge", MergeRequest{Run: r, Inputs: mergeInputs, Policy: step.Config["policy"].(string)}).Get(ctx, &ref)
		} else if step.Kind == "control" {
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
