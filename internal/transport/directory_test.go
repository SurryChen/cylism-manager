package transport

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransferWatchAllowsSlowProgressThenCancelsStall(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	var copied atomic.Int64
	const idleTimeout = 400 * time.Millisecond
	stop := watchTransferProgress(ctx, &copied, cancel, idleTimeout)
	defer stop()

	// Keep moving beyond the idle budget: total duration alone must not abort
	// a slow copy. Once bytes stop arriving, cancellation must unblock SSH I/O.
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	activeUntil := time.NewTimer(2 * idleTimeout)
	defer activeUntil.Stop()
active:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("progressing transfer was canceled: %v", context.Cause(ctx))
		case <-ticker.C:
			copied.Add(1)
		case <-activeUntil.C:
			break active
		}
	}
	select {
	case <-ctx.Done():
		if !strings.Contains(context.Cause(ctx).Error(), "无数据进展") {
			t.Fatalf("missing actionable stall reason: %v", context.Cause(ctx))
		}
	case <-time.After(5 * idleTimeout):
		t.Fatal("stalled transfer did not cancel")
	}
}

func TestTransferWatchPreservesParentDeadline(t *testing.T) {
	parent, finish := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer finish()
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	var copied atomic.Int64
	stop := watchTransferProgress(ctx, &copied, cancel, time.Minute)
	defer stop()
	select {
	case <-ctx.Done():
		if context.Cause(ctx) != context.DeadlineExceeded {
			t.Fatalf("parent deadline lost: %v", context.Cause(ctx))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent deadline did not cancel transfer")
	}
}
