// Runnelix phase-one development processes. Never expose the unauthenticated API publicly.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rushikeshg25/runnelix/internal/api"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/dispatch"
	"github.com/rushikeshg25/runnelix/internal/engine"
	"github.com/rushikeshg25/runnelix/internal/identity"
	"github.com/rushikeshg25/runnelix/internal/nodeservice"
	"github.com/rushikeshg25/runnelix/internal/scheduling"
	"github.com/rushikeshg25/runnelix/internal/secrets"
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
		return errors.New("usage: runnelix migrate|preflight|bootstrap|provision-owner|retention|retention-hold|api|node-service|worker|engine|dispatcher")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	address := flags.String("address", "127.0.0.1:8787", "development API loopback address")
	temporalAddress := flags.String("temporal-address", "127.0.0.1:7233", "Temporal address")
	workspaceID := flags.String("workspace", "workspace_dev", "development workspace ID")
	payloadDays := flags.Int("payload-days", 7, "terminal payload retention in elapsed days")
	historyDays := flags.Int("history-days", 30, "terminal history retention in elapsed days")
	batch := flags.Int("limit", 100, "maximum retention candidates (1-100)")
	apply := flags.Bool("apply", false, "apply retention; otherwise preview only")
	runID := flags.String("run", "", "run ID for retention hold")
	hold := flags.Bool("hold", true, "set or release a retention hold")
	ownerIssuer := flags.String("issuer", "", "owner OIDC issuer")
	ownerSubject := flags.String("subject", "", "owner OIDC subject")
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
	case "preflight":
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := s.CheckSchema(checkCtx); err != nil {
			return fmt.Errorf("database schema not ready: %w", err)
		}
		c, err := client.DialContext(checkCtx, client.Options{HostPort: *temporalAddress})
		if err != nil {
			return errors.New("Temporal not ready")
		}
		defer c.Close()
		if _, err = c.CheckHealth(checkCtx, &client.CheckHealthRequest{}); err != nil {
			return errors.New("Temporal health check failed")
		}
		fmt.Println("Database schema and Temporal ready; worker availability requires a smoke execution")
		return nil
	case "migrate":
		if s.Migrate(ctx) != nil {
			return errors.New("migration failed; inspect schema compatibility without recreating data")
		}
		fmt.Println("migrations applied")
		return nil
	case "provision-owner":
		if err := s.ProvisionOwner(store.WithAuditContext(ctx, "installation-operator", ""), *workspaceID, identity.Principal{Issuer: *ownerIssuer, Subject: *ownerSubject}); err != nil {
			return fmt.Errorf("owner provisioning failed: %w", err)
		}
		fmt.Println("workspace owner provisioned")
		return nil
	case "retention":
		result, err := s.RetainTerminalData(store.WithAuditContext(ctx, "installation-operator", ""), *workspaceID, store.RetentionPolicy{PayloadDays: *payloadDays, HistoryDays: *historyDays, Limit: *batch}, *apply)
		if err != nil {
			return fmt.Errorf("retention did not report success; inspect audit before retrying: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "retention-hold":
		if *runID == "" {
			return errors.New("retention-hold requires -run")
		}
		if err := s.SetRetentionHold(store.WithAuditContext(ctx, "installation-operator", ""), *workspaceID, *runID, *hold); err != nil {
			return err
		}
		fmt.Println("retention hold updated")
		return nil
	case "bootstrap":
		if s.CreateWorkspace(ctx, *workspaceID, "Development workspace") != nil {
			return errors.New("workspace initialization failed")
		}
		fmt.Println("development workspace ready")
		return nil
	case "api", "node-service":
		host, _, err := net.SplitHostPort(*address)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return errors.New("development API requires a literal loopback address")
		}
		var box *secrets.Box
		if key := os.Getenv("RUNNELIX_CREDENTIAL_KEY"); key != "" {
			box, err = secrets.New(key, "local-v1")
			if err != nil {
				return errors.New("invalid credential key")
			}
		}
		var verifier *identity.Verifier
		var browser *identity.Browser
		if os.Args[1] == "api" {
			config, configErr := identity.FromEnv(os.Getenv)
			if configErr != nil {
				return configErr
			}
			verifier, err = identity.New(ctx, config)
			if err != nil {
				return err
			}
			browser, err = identity.NewBrowser(verifier, os.Getenv("RUNNELIX_BROWSER_ORIGIN"), os.Getenv("RUNNELIX_OIDC_REDIRECT_URL"))
			if err != nil {
				return err
			}
		}
		handler := api.API{Store: s, Box: box, Identity: verifier, Browser: browser, BrowserOrigin: os.Getenv("RUNNELIX_BROWSER_ORIGIN")}.Handler()
		if os.Args[1] == "node-service" {
			hosts := map[string]bool{}
			for _, host := range strings.Split(os.Getenv("RUNNELIX_EGRESS_HOSTS"), ",") {
				if host != "" {
					hosts[strings.ToLower(strings.TrimSpace(host))] = true
				}
			}
			fixtures := map[string]string{}
			if raw := os.Getenv("RUNNELIX_DEV_PROVIDER_FIXTURES"); raw != "" {
				if definition.Decode([]byte(raw), &fixtures) != nil {
					return errors.New("invalid provider fixture mapping")
				}
			}
			handler, err = (nodeservice.Service{Store: s, Token: os.Getenv("RUNNELIX_WORKER_TOKEN"), Box: box, Egress: nodeservice.Egress{FixtureOrigins: fixtures, Hosts: hosts, AllowLoopback: os.Getenv("RUNNELIX_DEV_LOOPBACK_EGRESS") == "1"}}).Handler()
			if err != nil {
				return err
			}
		}
		srv := &http.Server{Addr: *address, Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
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
	case "worker", "engine", "dispatcher":
		c, err := client.DialContext(ctx, client.Options{HostPort: *temporalAddress})
		if err != nil {
			return errors.New("Temporal connection unavailable")
		}
		defer c.Close()
		if os.Args[1] == "engine" {
			w := worker.New(c, engine.Queue, worker.Options{})
			w.RegisterWorkflowWithOptions(engine.Execute, workflow.RegisterOptions{Name: engine.WorkflowName})
			w.RegisterActivity(&engine.Activities{Store: s})
			if err = w.Start(); err != nil {
				return errors.New("engine worker failed to start")
			}
			defer w.Stop()
			// Separate normal/sticky polling prevents a busy sticky queue from
			// starving new workflow tasks. Keep a small polling range per queue.
			v3 := worker.New(c, engine.QueueV3, worker.Options{
				WorkflowTaskPollerBehavior: worker.NewPollerBehaviorAutoscaling(worker.PollerBehaviorAutoscalingOptions{
					InitialNumberOfPollers: 1, MinimumNumberOfPollers: 1, MaximumNumberOfPollers: 2,
				}),
			})
			v3.RegisterWorkflowWithOptions(engine.ExecuteV3, workflow.RegisterOptions{Name: engine.WorkflowNameV3})
			v3.RegisterActivity(&engine.Activities{Store: s})
			if err = v3.Start(); err != nil {
				return errors.New("version 3 engine failed to start")
			}
			defer v3.Stop()
			schedules := worker.New(c, scheduling.Queue, worker.Options{})
			schedules.RegisterWorkflowWithOptions(scheduling.Admission, workflow.RegisterOptions{Name: scheduling.WorkflowName})
			schedules.RegisterActivityWithOptions(scheduling.Activity(s), activity.RegisterOptions{Name: "AdmitScheduleOccurrence"})
			if err = schedules.Start(); err != nil {
				return errors.New("schedule admission worker failed to start")
			}
			defer schedules.Stop()
			<-ctx.Done()
			return nil
		}
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
		dispatch.Run(ctx,
			func(ctx context.Context) {
				if err := dispatch.RepairProjection(ctx, s, c); err != nil && ctx.Err() == nil {
					log.Print("projection reconciliation incomplete; runtime remains authoritative")
				}
			},
			func(ctx context.Context) {
				if err := scheduling.Reconcile(ctx, s, c); err != nil && ctx.Err() == nil {
					log.Print("schedule reconciliation incomplete; retained for retry")
				}
			},
			func(ctx context.Context) {
				if err := dispatch.ReconcileResolutions(ctx, s, c); err != nil && ctx.Err() == nil {
					log.Print("resolution delivery incomplete; retained for retry")
				}
			},
			func(ctx context.Context) {
				if err := dispatch.ReconcileCancellation(ctx, s, c); err != nil && ctx.Err() == nil {
					log.Print("cancellation reconciliation incomplete; retained for retry")
				}
			},
			func(ctx context.Context) {
				for range 16 {
					worked, err := dispatch.Once(ctx, s, dispatch.TemporalStarter{Client: c})
					if err != nil && ctx.Err() == nil {
						log.Print("dispatch attempt incomplete; retained for retry")
					}
					if !worked || err != nil {
						break
					}
				}
			},
		)
		return nil
	default:
		return errors.New("unknown command")
	}
}
