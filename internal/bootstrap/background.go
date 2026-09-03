package bootstrap

import (
	"context"
	"sync"
	"time"
)

// BackgroundTasks owns cancellation and completion for Container-managed
// goroutines. Stop is idempotent and Wait returns after every task exits.
type BackgroundTasks struct {
	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func newBackgroundTasks(parent context.Context) *BackgroundTasks {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &BackgroundTasks{ctx: ctx, cancel: cancel}
}

// Go starts one lifecycle-bound background task.
func (t *BackgroundTasks) Go(task func(context.Context)) {
	if t == nil || task == nil {
		return
	}
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		task(t.ctx)
	}()
}

// Stop cancels all tasks. It may be called more than once.
func (t *BackgroundTasks) Stop() {
	if t == nil {
		return
	}
	t.stopOnce.Do(t.cancel)
}

// Wait blocks until every task started through Go has returned.
func (t *BackgroundTasks) Wait() {
	if t != nil {
		t.wg.Wait()
	}
}

// runPeriodic performs work immediately, then at each interval until the
// lifecycle context is cancelled.
func runPeriodic(ctx context.Context, interval time.Duration, run func(context.Context)) {
	if interval <= 0 {
		interval = time.Minute
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		run(ctx)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
	}
}
