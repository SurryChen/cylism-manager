package migration

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

type CopyControl interface {
	Wait(context.Context) error
	Paused() bool
	Progress(int64)
}

type migrationRun struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc
	mu      sync.Mutex
	control string
	changed chan struct{}
	paused  atomic.Bool
	copied  atomic.Int64
	onPause func(bool) error
}

func newMigrationRun(control string) *migrationRun {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &migrationRun{ctx: ctx, cancel: cancel, control: control, changed: make(chan struct{})}
}

func (r *migrationRun) request(control string) {
	r.mu.Lock()
	r.control = control
	close(r.changed)
	r.changed = make(chan struct{})
	r.mu.Unlock()
	if control == "cancel" {
		r.cancel(fmt.Errorf("用户取消迁移"))
	}
}

func (r *migrationRun) Paused() bool          { return r.paused.Load() }
func (r *migrationRun) Progress(copied int64) { r.copied.Store(copied) }

// Pause between reads without closing either SSH session or abandoning its tar.
// A process restart loses these streams; only an explicit rollback is safe then.
func (r *migrationRun) Wait(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return context.Cause(ctx)
		}
		r.mu.Lock()
		control, changed := r.control, r.changed
		r.mu.Unlock()
		if control == "cancel" {
			return fmt.Errorf("用户取消迁移")
		}
		paused := control == "pause"
		if r.paused.Swap(paused) != paused && r.onPause != nil {
			if err := r.onPause(paused); err != nil {
				return err
			}
		}
		if !paused {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-changed:
		}
	}
}

func cancellable(status string) bool {
	switch status {
	case "pending", "preflight", "provisioning_target", "stopping_source", "copying", "paused", "verifying", "rollback_failed":
		return true
	}
	return false
}

func (s *Service) Control(clusterID uint, component string, id uint, action string) (*model.ObservabilityMigration, error) {
	// Serialize requests with stage checkpoints: cancel must win before cutover,
	// or be rejected once the target may accept new writes.
	s.mu.Lock()
	item, err := s.repo.GetObservabilityMigration(id)
	if err != nil || item.ClusterID != clusterID || item.Component != component {
		s.mu.Unlock()
		return nil, fmt.Errorf("迁移任务不存在")
	}
	run := s.running[id]
	control, message := "", ""
	switch action {
	case "pause":
		if run == nil || item.Status != "copying" || item.Control != "" {
			err = fmt.Errorf("只有正在复制的数据可以暂停")
		} else {
			control, message = "pause", "已请求暂停传输；暂停期间源服务保持停写"
		}
	case "resume":
		if run == nil {
			err = fmt.Errorf("传输连接已中断，请取消并恢复源服务后重新迁移")
		} else if item.Control != "pause" || (item.Status != "copying" && item.Status != "paused") {
			err = fmt.Errorf("任务当前未暂停")
		} else {
			message = "已请求继续传输，沿用当前连接和已传输数据"
		}
	case "cancel":
		if !cancellable(item.Status) || item.Control == "cancel" {
			err = fmt.Errorf("当前阶段不能取消；切换后需保留目标实例的新写入")
		} else {
			control, message = "cancel", "已请求取消迁移；源组件已停写时将恢复源节点和原 PVC"
		}
	default:
		err = fmt.Errorf("不支持的迁移操作")
	}
	if err == nil {
		err = s.repo.SetObservabilityMigrationControl(id, control, message)
	}
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	item.Control = control
	if run != nil {
		run.request(control)
	}
	s.mu.Unlock()
	if run == nil && action == "cancel" {
		s.launch(*item)
	}
	s.capabilities(item)
	return item, nil
}

func (s *Service) capabilities(item *model.ObservabilityMigration) {
	s.mu.Lock()
	run := s.running[item.ID]
	alive := run != nil && run.ctx.Err() == nil
	s.mu.Unlock()
	item.CanPause = alive && item.Status == "copying" && item.Control == ""
	item.CanResume = alive && item.Control == "pause" && (item.Status == "copying" || item.Status == "paused")
	item.CanCancel = cancellable(item.Status) && item.Control != "cancel"
}

func (s *Service) reportCopy(item *model.ObservabilityMigration, run *migrationRun) func() {
	stop, done := make(chan struct{}), make(chan struct{})
	go func(id uint, total int64) {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		previous, at := run.copied.Load(), time.Now()
		for {
			select {
			case <-stop:
				return
			case <-run.ctx.Done():
				return
			case now := <-ticker.C:
				copied := run.copied.Load()
				rate := int64(float64(copied-previous) / now.Sub(at).Seconds())
				if run.Paused() {
					rate = 0
				}
				if err := s.repo.UpdateObservabilityMigrationProgress(id, total, copied, rate); err != nil {
					log.Printf("记录迁移 #%d 复制进度失败: %v", id, err)
				}
				previous, at = copied, now
			}
		}
	}(item.ID, item.BytesTotal)
	return func() { close(stop); <-done }
}
