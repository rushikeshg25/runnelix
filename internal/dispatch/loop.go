package dispatch

import (
	"context"
	"sync"
	"time"
)

// Run repeats each job independently every 250ms. A slow reconciliation must
// not block admission dispatch or other maintenance. Calls to the same job
// never overlap. Jobs must honor ctx; Run waits for all of them on cancellation.
func Run(ctx context.Context, jobs ...func(context.Context)) {
	var group sync.WaitGroup
	for _, job := range jobs {
		group.Add(1)
		go func() {
			defer group.Done()
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if ctx.Err() != nil {
						return
					}
					job(ctx)
				}
			}
		}()
	}
	group.Wait()
}
