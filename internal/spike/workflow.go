package spike

import (
	"errors"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const WorkflowName = "RunnelixSpike"
const ActivityName = "RunNode"

type State struct {
	Status      string `json:"status"`
	NodeVersion string `json:"nodeVersion"`
}

// Execute proves a single action boundary. It is not the product DAG engine.
func Execute(ctx workflow.Context, req Request) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, temporal.NewNonRetryableApplicationError("invalid invocation", "InvalidInput", err)
	}
	state := State{Status: "running", NodeVersion: req.NodeVersion}
	if err := workflow.SetQueryHandler(ctx, "state", func() (State, error) { return state, nil }); err != nil {
		return Result{}, err
	}
	if req.WaitMS > 0 {
		state.Status = "waiting"
		if err := workflow.Sleep(ctx, time.Duration(req.WaitMS)*time.Millisecond); err != nil {
			return Result{}, err
		}
	}
	state.Status = "executing"
	maximum := int32(3)
	if req.Mode == "unsafe" {
		maximum = 1
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue:              NodeQueue(req.NodeVersion),
		StartToCloseTimeout:    3 * time.Second,
		ScheduleToCloseTimeout: 15 * time.Second,
		HeartbeatTimeout:       time.Second,
		WaitForCancellation:    true,
		RetryPolicy:            &temporal.RetryPolicy{InitialInterval: 100 * time.Millisecond, MaximumInterval: 500 * time.Millisecond, MaximumAttempts: maximum},
	})
	var result Result
	err := workflow.ExecuteActivity(ctx, ActivityName, req).Get(ctx, &result)
	if err != nil {
		if temporal.IsCanceledError(err) {
			return Result{}, err
		}
		var app *temporal.ApplicationError
		knownPreSend := errors.As(err, &app) && (app.Type() == "InvalidInput" || app.Type() == "VersionMismatch")
		if req.Mode == "unsafe" && !knownPreSend {
			state.Status = "needs_attention"
			return Result{Status: "needs_attention", WorkerVersion: req.NodeVersion, Attempts: 1, Value: map[string]any{}}, nil
		}
		return Result{}, err
	}
	if result.Status != "succeeded" || result.WorkerVersion != req.NodeVersion || result.ActionID == "" {
		return Result{}, temporal.NewNonRetryableApplicationError("invalid action result", "InvalidResult", nil)
	}
	state.Status = "succeeded"
	return result, nil
}
