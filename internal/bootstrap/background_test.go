package bootstrap

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackgroundTasksRunImmediatelyAndStopIdempotently(t *testing.T) {
	tasks := newBackgroundTasks(context.Background())
	var runs atomic.Int32
	started := make(chan struct{})
	tasks.Go(func(ctx context.Context) {
		runPeriodic(ctx, time.Hour, func(context.Context) {
			if runs.Add(1) == 1 {
				close(started)
			}
		})
	})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background task did not execute immediately")
	}
	tasks.Stop()
	tasks.Stop()
	tasks.Wait()
	if runs.Load() != 1 {
		t.Fatalf("runs = %d, want one immediate execution", runs.Load())
	}
}

func TestContainerStartBackgroundStopsExistingTasks(t *testing.T) {
	container := &Container{}
	first := container.StartBackground(context.Background(), BackgroundConfig{})
	second := container.StartBackground(context.Background(), BackgroundConfig{})
	select {
	case <-first.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("starting replacement tasks did not stop the previous controller")
	}
	if container.background != second {
		t.Fatal("container did not retain the replacement background controller")
	}
	second.Stop()
	second.Wait()
}
