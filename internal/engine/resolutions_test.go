package engine

import (
	"context"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/store"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"testing"
	"time"
)

func TestUncertainActionResumesOnlyFromRecordedDecision(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	calls := 0
	envelope := definition.Envelope{InvocationID: "invocation_post"}
	env.RegisterActivityWithOptions(func(context.Context, ScopedRequest) (definition.Envelope, error) { return envelope, nil }, activity.RegisterOptions{Name: "PrepareScoped"})
	env.RegisterActivityWithOptions(func(context.Context, definition.Envelope) (Completion, error) {
		calls++
		return Completion{}, temporal.NewNonRetryableApplicationError("Unknown provider outcome", "ProviderNetwork", nil)
	}, activity.RegisterOptions{Name: NodeActivity})
	env.RegisterActivityWithOptions(func(context.Context, definition.Envelope) error { return nil }, activity.RegisterOptions{Name: "MarkUncertain"})
	env.RegisterActivityWithOptions(func(context.Context, definition.Envelope) (store.Resolution, error) {
		return store.Resolution{Action: "confirm", OutputRef: "verified_output"}, nil
	}, activity.RegisterOptions{Name: "Resolve"})
	env.RegisterDelayedCallback(func() { env.SignalWorkflow("resolve/invocation_post", true) }, time.Second)
	env.ExecuteWorkflow(func(ctx workflow.Context) (string, error) {
		ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: time.Second})
		return actionV3(ctx, StepRequest{Step: definition.Step{Node: definition.Node{ID: "post"}, RetrySafety: "unsafe"}}, "root", func(workflow.Context, store.Event) error { return nil })
	})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	var result string
	if err := env.GetWorkflowResult(&result); err != nil || result != "verified_output" {
		t.Fatal(result, err)
	}
	if calls != 1 {
		t.Fatal("operator decision retried provider", calls)
	}
}
