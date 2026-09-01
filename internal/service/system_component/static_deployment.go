package system_component

import (
	"context"
	"fmt"

	"github.com/cylism/cylism-manager/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type StaticDeploymentAdapter interface {
	GetDeployment(context.Context, string, string) (*appsv1.Deployment, error)
	GetNodeInfo(context.Context, string) (*k8s.NodeInfo, error)
	HasDynamicClient() bool
	DeleteHelmChartConfig(context.Context, string, string) error
	ApplyStaticDeploymentConfig(context.Context, string, string, k8s.StaticDeploymentConfig) error
	ListNodes(context.Context) ([]corev1.Node, error)
	ListPods(context.Context, string, string) ([]corev1.Pod, error)
}

type StaticDeploymentService struct{ Adapter StaticDeploymentAdapter }

func ValidateNode(ctx context.Context, adapter interface {
	GetNodeInfo(context.Context, string) (*k8s.NodeInfo, error)
}, nodeName string) error {
	if nodeName == "" {
		return nil
	}
	node, err := adapter.GetNodeInfo(ctx, nodeName)
	if err != nil || node == nil {
		return fmt.Errorf("部署节点不存在或未加入集群")
	}
	if !node.Ready || node.Evicted {
		return fmt.Errorf("部署节点未就绪或已禁止调度")
	}
	return nil
}

func (s StaticDeploymentService) Apply(ctx context.Context, namespace, name, values string) error {
	if s.Adapter == nil {
		return fmt.Errorf("Kubernetes 集群未连接")
	}
	cfg, err := ParseStaticDeploymentConfig(values, name)
	if err != nil {
		return err
	}
	if err := ValidateNode(ctx, s.Adapter, cfg.NodeName); err != nil {
		return err
	}
	p := profile(name)
	d, e := s.Adapter.GetDeployment(ctx, namespace, name)
	if e != nil {
		return fmt.Errorf("读取 %s 当前副本数失败: %w", name, e)
	}
	current := p.DefaultReplicas
	if d.Spec.Replicas != nil {
		current = *d.Spec.Replicas
	}
	if !p.SupportsHA && cfg.Replicas != current {
		return fmt.Errorf("%s 副本由 K3s/组件 profile 管理，平台不支持修改为 %d 副本", name, cfg.Replicas)
	}
	if cfg.NodeName != "" && !p.SupportsNodePlacement {
		return fmt.Errorf("%s 不支持通过平台固定部署节点", name)
	}
	if p.SupportsHA && cfg.Replicas > current {
		if err := ValidateHAIncrease(ctx, s.Adapter, name, namespace, d.Status.ReadyReplicas, d.Status.AvailableReplicas, cfg.Replicas, cfg.NodeName); err != nil {
			return err
		}
	}
	if s.Adapter.HasDynamicClient() {
		if err := s.Adapter.DeleteHelmChartConfig(ctx, namespace, name); err != nil {
			return err
		}
	}
	return s.Adapter.ApplyStaticDeploymentConfig(ctx, namespace, name, cfg)
}
