package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/rushikeshg25/runnelix/internal/spike"
	enums "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	sdklog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: spike worker|start|result|query|cancel|describe [flags]")
	}
	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	address := flags.String("address", "127.0.0.1:7233", "Temporal address")
	version := flags.String("version", "v1", "workflow worker version")
	input := flags.String("input", "", "request JSON file")
	id := flags.String("id", "", "workflow/run ID")
	timeout := flags.Duration("timeout", 60*time.Second, "client operation deadline")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	c, err := client.DialContext(ctx, client.Options{HostPort: *address, Logger: sdklog.NewStructuredLogger(slog.New(slog.NewTextHandler(os.Stderr, nil)))})
	if err != nil {
		return err
	}
	defer c.Close()
	var output any
	switch command {
	case "worker":
		if *version != "v1" && *version != "v2" {
			return fmt.Errorf("unsupported worker version")
		}
		w := worker.New(c, spike.WorkflowQueue(*version), worker.Options{})
		w.RegisterWorkflowWithOptions(spike.Execute, workflow.RegisterOptions{Name: spike.WorkflowName})
		return w.Run(worker.InterruptCh())
	case "start":
		b, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		r, err := spike.DecodeRequest(b)
		if err != nil {
			return err
		}
		w, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: r.RunID, TaskQueue: spike.WorkflowQueue(r.NodeVersion), WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowExecutionTimeout: 2 * time.Minute}, spike.WorkflowName, r)
		if err != nil {
			return err
		}
		output = map[string]string{"id": w.GetID(), "executionId": w.GetRunID()}
	case "result":
		var result spike.Result
		if err := c.GetWorkflow(ctx, *id, "").Get(ctx, &result); err != nil {
			return err
		}
		output = result
	case "query":
		value, err := c.QueryWorkflow(ctx, *id, "", "state")
		if err != nil {
			return err
		}
		var state spike.State
		if err := value.Get(&state); err != nil {
			return err
		}
		output = state
	case "cancel":
		if err := c.CancelWorkflow(ctx, *id, ""); err != nil {
			return err
		}
		output = map[string]bool{"requested": true}
	case "describe":
		description, err := c.DescribeWorkflowExecution(ctx, *id, "")
		if err != nil {
			return err
		}
		output = map[string]string{"status": description.WorkflowExecutionInfo.Status.String()}
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	return json.NewEncoder(os.Stdout).Encode(output)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
