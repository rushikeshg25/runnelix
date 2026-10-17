package dispatch

import (
	"context"
	"github.com/rushikeshg25/runnelix/internal/store"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"strings"
	"testing"
)

func TestReceiptWorkflowIdentity(t *testing.T) {
	for _, valid := range []bool{true, false} {
		var suite testsuite.WorkflowTestSuite
		env := suite.NewTestWorkflowEnvironment()
		request := store.StartRequest{WorkspaceID: "workspace_test", RunID: "run_test", VersionID: "version_test", PlanDigest: strings.Repeat("a", 64)}
		id := store.RuntimeID(request.RunID)
		if !valid {
			id = "wrong"
		}
		env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: id})
		env.RegisterActivityWithOptions(func(context.Context, store.StartRequest) error { return nil }, activity.RegisterOptions{Name: ReceiptActivity})
		if valid {
			env.OnActivity(ReceiptActivity, mock.Anything, request).Return(nil).Once()
		}
		env.ExecuteWorkflow(Admit, request)
		if (env.GetWorkflowError() == nil) != valid {
			t.Fatal("identity validation failed", env.GetWorkflowError())
		}
		env.AssertExpectations(t)
	}
}
