package system_component

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

type KubernetesAdapter interface {
	Available() bool
	GetDeployment(context.Context, string, string) (*appsv1.Deployment, error)
	ListNodes(context.Context) ([]corev1.Node, error)
	ListPods(context.Context, string, string) ([]corev1.Pod, error)
	HasDynamicClient() bool
	DetectSystemComponent(context.Context, string, string) (k8s.SystemComponentDetection, error)
	GetNodeInfo(context.Context, string) (*k8s.NodeInfo, error)
	ApplyHelmChartConfig(context.Context, string, string, string) error
	DeleteHelmChartConfig(context.Context, string, string) error
	ApplyStaticDeploymentConfig(context.Context, string, string, k8s.StaticDeploymentConfig) error
	RestoreStaticDeploymentDefaults(context.Context, string, string) error
}

type WorkflowError struct {
	Kind string
	Err  error
}

func (e *WorkflowError) Error() string { return e.Err.Error() }
func workflow(kind string, err error) error {
	if err == nil {
		return nil
	}
	return &WorkflowError{Kind: kind, Err: err}
}

type UpdateResult struct{ Config *model.SystemComponentConfig }

// Update owns validation, control-source detection, mode-specific apply and persistence.
func Update(ctx context.Context, repo UpdateRepository, adapter KubernetesAdapter, chart, values, timeout string, userID uint, now time.Time) (*UpdateResult, error) {
	ns, ok := Namespace(chart)
	if !ok {
		return nil, workflow("validation", fmt.Errorf("不支持该系统组件"))
	}
	values = strings.TrimSpace(values)
	if values == "" {
		return nil, workflow("validation", fmt.Errorf("values 配置不能为空"))
	}
	if timeout != "" {
		if chart != "traefik" {
			return nil, workflow("validation", fmt.Errorf("仅 Traefik 支持入口请求读取超时"))
		}
		n, err := NormalizeReadTimeout(timeout)
		if err != nil {
			return nil, workflow("validation", err)
		}
		values, err = RenderTraefikReadTimeout(values, n)
		if err != nil {
			return nil, workflow("validation", fmt.Errorf("Traefik 配置无效: %w", err))
		}
	}
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(values), &raw); err != nil {
		return nil, workflow("validation", fmt.Errorf("values 配置不是合法 YAML: %w", err))
	}
	if adapter == nil || !adapter.Available() {
		return nil, workflow("unavailable", fmt.Errorf("Kubernetes 集群未连接"))
	}
	detection, err := adapter.DetectSystemComponent(ctx, ns, chart)
	if err != nil {
		return nil, workflow("k8s", fmt.Errorf("检测系统组件控制源失败: %w", err))
	}
	if detection.Mode == k8s.EmbeddedMode {
		return nil, workflow("conflict", fmt.Errorf("该系统组件由 K3s 内置控制器管理，不支持通过平台修改"))
	}
	if detection.Mode == k8s.UnknownMode {
		return nil, workflow("conflict", fmt.Errorf("无法识别该系统组件的控制源，已拒绝写入"))
	}
	nowCopy := now
	cfg := &model.SystemComponentConfig{ChartName: chart, Namespace: ns, ControllerMode: string(detection.Mode), ValuesContent: values, Enabled: true, LastAppliedAt: &nowCopy, CreatedBy: userID}
	var applyErr error
	switch detection.Mode {
	case k8s.HelmChartMode:
		if !adapter.HasDynamicClient() {
			applyErr = fmt.Errorf("Kubernetes 动态客户端未初始化")
		} else {
			applyErr = adapter.ApplyHelmChartConfig(ctx, ns, chart, values)
		}
	case k8s.StaticDeploymentMode:
		ss := StaticDeploymentService{Adapter: adapter}
		applyErr = ss.Apply(ctx, ns, chart, values)
	default:
		applyErr = fmt.Errorf("不支持的系统组件控制模式: %s", detection.Mode)
	}
	if applyErr != nil {
		cfg.ApplyStatus, cfg.ApplyError = "failed", applyErr.Error()
		if repo != nil {
			_ = repo.UpsertSystemComponentConfig(cfg)
		}
		return nil, workflow("k8s", applyErr)
	}
	cfg.ApplyStatus, cfg.ApplyError = "succeeded", ""
	if repo != nil {
		if err := repo.UpsertSystemComponentConfig(cfg); err != nil {
			return nil, workflow("db", fmt.Errorf("保存系统组件配置失败: %w", err))
		}
	}
	return &UpdateResult{Config: cfg}, nil
}

func RevertManaged(ctx context.Context, repo RevertRepository, adapter KubernetesAdapter, chart string) error {
	ns, ok := Namespace(chart)
	if !ok {
		return workflow("validation", fmt.Errorf("不支持该系统组件"))
	}
	if adapter == nil || !adapter.Available() {
		return workflow("unavailable", fmt.Errorf("Kubernetes 集群未连接"))
	}
	detection, err := adapter.DetectSystemComponent(ctx, ns, chart)
	if err != nil {
		return workflow("k8s", fmt.Errorf("检测系统组件控制源失败: %w", err))
	}
	if detection.Mode == k8s.HelmChartMode && !adapter.HasDynamicClient() {
		return workflow("conflict", fmt.Errorf("Kubernetes 动态客户端未初始化"))
	}
	return Revert(ctx, repo, adapter, chart, detection.Mode)
}

type UpdateRepository interface {
	UpsertSystemComponentConfig(*model.SystemComponentConfig) error
}
type UpdateAdapter interface{ Adapter }
type RevertRepository interface{ DeleteSystemComponentConfig(string) error }

func Revert(ctx context.Context, repo RevertRepository, adapter Adapter, chart string, mode k8s.ControllerMode) error {
	if adapter == nil {
		return fmt.Errorf("系统组件更新依赖不可用")
	}
	var err error
	switch mode {
	case k8s.HelmChartMode:
		err = Restore(ctx, adapter, chart, false)
	case k8s.StaticDeploymentMode:
		err = Restore(ctx, adapter, chart, false)
		if err == nil {
			err = Restore(ctx, adapter, chart, true)
		}
	default:
		return fmt.Errorf("该系统组件当前不支持恢复默认配置")
	}
	if err != nil {
		return err
	}
	if repo != nil {
		return repo.DeleteSystemComponentConfig(chart)
	}
	return nil
}

// ApplyUpdate performs the mode-specific Kubernetes apply and persists the
// resulting status. HTTP handlers only need to validate DTOs and build the
// typed static deployment config.
func ApplyUpdate(ctx context.Context, repo UpdateRepository, adapter UpdateAdapter, config *model.SystemComponentConfig, mode k8s.ControllerMode, static *k8s.StaticDeploymentConfig) error {
	if config == nil || adapter == nil {
		return fmt.Errorf("系统组件更新依赖不可用")
	}
	var err error
	if mode == k8s.StaticDeploymentMode {
		if static == nil {
			return fmt.Errorf("静态 Deployment 配置不能为空")
		}
		err = adapter.ApplyStaticDeploymentConfig(ctx, config.Namespace, config.ChartName, *static)
	} else if mode == k8s.HelmChartMode {
		err = adapter.ApplyHelmChartConfig(ctx, config.Namespace, config.ChartName, config.ValuesContent)
	} else {
		return fmt.Errorf("不支持的系统组件控制模式: %s", mode)
	}
	if err != nil {
		config.ApplyStatus, config.ApplyError = "failed", err.Error()
		_ = repo.UpsertSystemComponentConfig(config)
		return err
	}
	config.ApplyStatus, config.ApplyError = "succeeded", ""
	return repo.UpsertSystemComponentConfig(config)
}

type Adapter interface {
	ApplyHelmChartConfig(context.Context, string, string, string) error
	DeleteHelmChartConfig(context.Context, string, string) error
	ApplyStaticDeploymentConfig(context.Context, string, string, k8s.StaticDeploymentConfig) error
	RestoreStaticDeploymentDefaults(context.Context, string, string) error
}

func Apply(ctx context.Context, adapter Adapter, chart, values string, static *k8s.StaticDeploymentConfig) error {
	if adapter == nil {
		return fmt.Errorf("系统组件适配器不可用")
	}
	ns, ok := Namespace(chart)
	if !ok {
		return fmt.Errorf("不支持该系统组件")
	}
	if static != nil {
		return adapter.ApplyStaticDeploymentConfig(ctx, ns, chart, *static)
	}
	return adapter.ApplyHelmChartConfig(ctx, ns, chart, values)
}
func Restore(ctx context.Context, adapter Adapter, chart string, static bool) error {
	if adapter == nil {
		return fmt.Errorf("系统组件适配器不可用")
	}
	ns, ok := Namespace(chart)
	if !ok {
		return fmt.Errorf("不支持该系统组件")
	}
	if static {
		return adapter.RestoreStaticDeploymentDefaults(ctx, ns, chart)
	}
	return adapter.DeleteHelmChartConfig(ctx, ns, chart)
}
