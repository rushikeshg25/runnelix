// Runnelix phase-one development processes. Never expose the unauthenticated API publicly.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rushikeshg25/runnelix/internal/api"
	"github.com/rushikeshg25/runnelix/internal/dispatch"
	"github.com/rushikeshg25/runnelix/internal/store"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: runnelix migrate|bootstrap|api|worker|dispatcher")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	address := flags.String("address", "127.0.0.1:8787", "development API loopback address")
	temporalAddress := flags.String("temporal-address", "127.0.0.1:7233", "Temporal address")
	workspaceID := flags.String("workspace", "workspace_dev", "development workspace ID")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dsn := os.Getenv("RUNNELIX_DATABASE_URL")
	if dsn == "" {
		return errors.New("RUNNELIX_DATABASE_URL is required")
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s, err := store.Open(connectCtx, dsn)
	if err != nil {
		return err
	}
	defer s.Close()
	switch os.Args[1] {
	case "migrate":
		if s.Migrate(ctx) != nil {
			return errors.New("migration failed; inspect schema compatibility without recreating data")
		}
		fmt.Println("migrations applied")
		return nil
	case "bootstrap":
		if s.CreateWorkspace(ctx, *workspaceID, "Development workspace") != nil {
			return errors.New("workspace initialization failed")
		}
		fmt.Println("development workspace ready")
		return nil
	case "api":
		host, _, err := net.SplitHostPort(*address)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return errors.New("development API requires a literal loopback address")
		}
		srv := &http.Server{Addr: *address, Handler: api.API{Store: s}.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = srv.Shutdown(shutdown)
			case <-done:
			}
		}()
		defer close(done)
		log.Print("development API starting on loopback")
		err = srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("API listener stopped unexpectedly")
	case "worker", "dispatcher":
		c, err := client.DialContext(ctx, client.Options{HostPort: *temporalAddress})
		if err != nil {
			return errors.New("Temporal connection unavailable")
		}
		defer c.Close()
		if os.Args[1] == "worker" {
			w := worker.New(c, dispatch.Queue, worker.Options{})
			w.RegisterWorkflowWithOptions(dispatch.Admit, workflow.RegisterOptions{Name: dispatch.WorkflowName})
			w.RegisterActivityWithOptions(dispatch.Receipt(s), activity.RegisterOptions{Name: dispatch.ReceiptActivity})
			if err = w.Start(); err != nil {
				return errors.New("admission worker failed to start")
			}
			defer w.Stop()
			<-ctx.Done()
			return nil
		}
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				_, err := dispatch.Once(ctx, s, dispatch.TemporalStarter{Client: c})
				if err != nil && ctx.Err() == nil {
					log.Print("dispatch attempt incomplete; retained for retry")
				}
			}
		}
	default:
		return errors.New("unknown command")
	}
}
