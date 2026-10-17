// Test-only crash checkpoint process. Never part of the application binary.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/rushikeshg25/runnelix/internal/dispatch"
	"github.com/rushikeshg25/runnelix/internal/store"
	"go.temporal.io/sdk/client"
	"os"
	"time"
)

type forbiddenStarter struct{}

func (forbiddenStarter) Start(context.Context, store.StartRequest) error {
	panic("durable receipt must prevent another start call")
}
func main() {
	mode := flag.String("mode", "before-start", "checkpoint")
	path := flag.String("checkpoint", "", "checkpoint file")
	run := flag.String("run", "", "expected candidate")
	address := flag.String("address", "127.0.0.1:7233", "Temporal address")
	flag.Parse()
	ctx := context.Background()
	s, err := store.Open(ctx, os.Getenv("RUNNELIX_DATABASE_URL"))
	must(err)
	defer s.Close()
	if *mode == "reconcile" {
		_, err = dispatch.Once(ctx, s, forbiddenStarter{})
		must(err)
		return
	}
	claim, err := s.Claim(ctx, 2*time.Second)
	must(err)
	if claim.RunID != *run {
		panic("unexpected candidate")
	}
	if *mode != "before-start" {
		c, err := client.Dial(client.Options{HostPort: *address})
		must(err)
		defer c.Close()
		must((dispatch.TemporalStarter{Client: c}).Start(ctx, claim.StartRequest))
	}
	if *mode == "after-receipt" {
		deadline := time.Now().Add(15 * time.Second)
		for {
			present, err := s.HasReceipt(ctx, claim.StartRequest)
			must(err)
			if present {
				break
			}
			if time.Now().After(deadline) {
				panic("receipt deadline")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	encoded, err := json.Marshal(map[string]string{"runId": claim.RunID, "checkpoint": *mode})
	must(err)
	must(os.WriteFile(*path, encoded, 0600))
	for {
		time.Sleep(time.Second)
	}
}
func must(err error) {
	if err != nil {
		panic(errors.New("phase1 probe failed; inspect owned fixture state"))
	}
}
