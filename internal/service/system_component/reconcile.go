package system_component

import (
	"context"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"time"
)

type ConfigRepository interface {
	ListSystemComponentConfigs() ([]model.SystemComponentConfig, error)
	UpsertSystemComponentConfig(*model.SystemComponentConfig) error
}
type Detector interface {
	DetectSystemComponent(context.Context, string, string) (k8s.SystemComponentDetection, error)
	ApplyStaticDeploymentConfig(context.Context, string, string, k8s.StaticDeploymentConfig) error
}
type StaticParser func(string, string) (k8s.StaticDeploymentConfig, error)
type NodeValidator func(context.Context, string) error

func Reconcile(ctx context.Context, repo ConfigRepository, detector Detector, parse StaticParser, validate NodeValidator, now time.Time) error {
	configs, err := repo.ListSystemComponentConfigs()
	if err != nil {
		return err
	}
	for i := range configs {
		c := &configs[i]
		if !c.Enabled {
			continue
		}
		detection, detectErr := detector.DetectSystemComponent(ctx, c.Namespace, c.ChartName)
		if detectErr != nil {
			c.ApplyStatus, c.ApplyError = "failed", detectErr.Error()
		} else if c.ControllerMode != "" && c.ControllerMode != string(detection.Mode) {
			c.ApplyStatus, c.ApplyError = "failed", "组件控制源已变更为 "+string(detection.Mode)+"，已停止自动重放"
		} else if detection.Mode == k8s.StaticDeploymentMode {
			parsed, parseErr := parse(c.ValuesContent, c.ChartName)
			if parseErr == nil && parsed.NodeName != "" {
				parseErr = validate(ctx, parsed.NodeName)
			}
			if parseErr == nil {
				parseErr = detector.ApplyStaticDeploymentConfig(ctx, c.Namespace, c.ChartName, parsed)
			}
			if parseErr != nil {
				c.ApplyStatus, c.ApplyError = "failed", parseErr.Error()
			} else {
				c.ApplyStatus, c.ApplyError = "succeeded", ""
				c.LastAppliedAt = &now
			}
		}
		if detectErr == nil && (c.ControllerMode == "" || c.ControllerMode == string(detection.Mode)) {
			c.ControllerMode = string(detection.Mode)
		}
		_ = repo.UpsertSystemComponentConfig(c)
	}
	return nil
}

// Run keeps reconciliation lifecycle outside HTTP handlers. It performs an
// immediate pass and then waits for the supplied interval until cancellation.
func Run(ctx context.Context, interval time.Duration, repo ConfigRepository, detector Detector, parse StaticParser, validate NodeValidator, now func() time.Time) error {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if now == nil {
		now = time.Now
	}
	for {
		if err := Reconcile(ctx, repo, detector, parse, validate, now()); err != nil {
			// A transient repository/Kubernetes failure should not terminate the
			// background loop; the next tick retries it.
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
