package spike

import (
	"context"
	"testing"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

func request() Request {
	return Request{ContractVersion: 1, RunID: "test-run", NodeVersion: "v1", Mode: "safe", Fault: "none", Value: map[string]any{"text": "ok"}}
}

func TestWorkflowRetryPolicy(t *testing.T) {
	for _, mode := range []string{"safe", "unsafe"} {
		t.Run(mode, func(t *testing.T) {
			var suite testsuite.WorkflowTestSuite
			env := suite.NewTestWorkflowEnvironment()
			attempts := 0
			env.RegisterActivityWithOptions(func(ctx context.Context, r Request) (Result, error) {
				attempts++
				if attempts == 1 {
					return Result{}, temporal.NewApplicationError("lost response", "Network")
				}
				return Result{Status: "succeeded", ActionID: "fake-1", WorkerVersion: r.NodeVersion, Attempts: attempts, Value: r.Value}, nil
			}, activity.RegisterOptions{Name: ActivityName})
			r := request()
			r.Mode = mode
			env.ExecuteWorkflow(Execute, r)
			if err := env.GetWorkflowError(); err != nil {
				t.Fatal(err)
			}
			var result Result
			if err := env.GetWorkflowResult(&result); err != nil {
				t.Fatal(err)
			}
			if mode == "safe" && (result.Status != "succeeded" || attempts != 2) {
				t.Fatalf("safe: %+v attempts=%d", result, attempts)
			}
			if mode == "unsafe" && (result.Status != "needs_attention" || attempts != 1) {
				t.Fatalf("unsafe: %+v attempts=%d", result, attempts)
			}
		})
	}
}

func TestCancellationDuringDurableWaitDoesNotInvokeAction(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	r := request()
	r.WaitMS = 60000
	env.RegisterDelayedCallback(func() { env.CancelWorkflow() }, time.Second)
	env.ExecuteWorkflow(Execute, r)
	if !temporal.IsCanceledError(env.GetWorkflowError()) {
		t.Fatalf("expected cancellation, got %v", env.GetWorkflowError())
	}
}

func TestPreSendFailureIsNotUncertain(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(func(context.Context, Request) (Result, error) {
		return Result{}, temporal.NewNonRetryableApplicationError("bad version", "VersionMismatch", nil)
	}, activity.RegisterOptions{Name: ActivityName})
	r := request()
	r.Mode = "unsafe"
	env.ExecuteWorkflow(Execute, r)
	if env.GetWorkflowError() == nil {
		t.Fatal("expected known pre-send failure")
	}
}
