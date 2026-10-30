package dispatch

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestSlowMaintenanceDoesNotBlockDispatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var maintenance, starts atomic.Int32
		done := make(chan struct{})
		go func() {
			Run(ctx, func(ctx context.Context) { maintenance.Add(1); <-ctx.Done() },
				func(context.Context) { starts.Add(1) })
			close(done)
		}()
		time.Sleep(time.Second)
		synctest.Wait()
		if maintenance.Load() != 1 {
			t.Errorf("maintenance overlapped: %d calls", maintenance.Load())
		}
		if starts.Load() < 3 {
			t.Errorf("slow maintenance blocked dispatch: %d cycles", starts.Load())
		}
		cancel()
		<-done
	})
}

func TestPeriodicJobsStopAndJoinOnCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var calls, finished atomic.Int32
		done := make(chan struct{})
		go func() {
			Run(ctx, func(ctx context.Context) {
				calls.Add(1)
				<-ctx.Done()
				time.Sleep(time.Second)
				finished.Add(1)
			})
			close(done)
		}()
		time.Sleep(time.Second)
		cancel()
		<-done
		if calls.Load() != 1 || finished.Load() != 1 {
			t.Fatalf("run returned before job exited: calls=%d finished=%d", calls.Load(), finished.Load())
		}
		time.Sleep(time.Second)
		if calls.Load() != 1 {
			t.Fatal("job ran after cancellation")
		}
	})
}
