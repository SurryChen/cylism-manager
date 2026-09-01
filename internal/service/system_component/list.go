package system_component

import (
	"context"
	"fmt"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

type ListRepository interface {
	ListSystemComponentConfigs() ([]model.SystemComponentConfig, error)
}
type ListAdapter interface {
	DetectSystemComponent(context.Context, string, string) (k8s.SystemComponentDetection, error)
}
type AvailabilityProfile struct {
	DefaultReplicas                   int32
	SupportsHA, SupportsNodePlacement bool
	Description                       string
}

var availabilityProfiles = map[string]AvailabilityProfile{
	"coredns":                {DefaultReplicas: 1, SupportsHA: true, SupportsNodePlacement: true, Description: "支持高可用；启用前需要至少两个可调度节点。"},
	"metrics-server":         {DefaultReplicas: 1, Description: "副本由 K3s 管理，平台不提供通用双副本基线。"},
	"local-path-provisioner": {DefaultReplicas: 1, Description: "保持单个活动 provisioner，平台不提供通用双副本基线。"},
}

func profile(chart string) AvailabilityProfile {
	if p, ok := availabilityProfiles[chart]; ok {
		return p
	}
	return AvailabilityProfile{Description: "未定义高可用 profile，副本策略由组件自身管理。"}
}
func capabilities(chart string, mode k8s.ControllerMode) map[string]interface{} {
	p := profile(chart)
	static := mode == k8s.StaticDeploymentMode
	return map[string]interface{}{"configure": mode == k8s.HelmChartMode || static, "node_placement": static && p.SupportsNodePlacement, "rollout": static, "replica_scaling": static && p.SupportsHA, "safe_baseline": static && p.SupportsHA, "restore": mode == k8s.HelmChartMode || static}
}

type ComponentListService struct {
	Repo    ListRepository
	Adapter interface {
		ListAdapter
		Available() bool
	}
}

func (s ComponentListService) List(ctx context.Context) ([]map[string]interface{}, error) {
	if s.Adapter == nil || !s.Adapter.Available() {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	configs := map[string]*model.SystemComponentConfig{}
	if s.Repo != nil {
		if rows, err := s.Repo.ListSystemComponentConfigs(); err == nil {
			for i := range rows {
				configs[rows[i].ChartName] = &rows[i]
			}
		}
	}
	result := make([]map[string]interface{}, 0, len(Charts()))
	for chart, ns := range Charts() {
		item := map[string]interface{}{"chart_name": chart, "namespace": ns, "values_content": "", "enabled": false, "apply_status": "", "apply_error": "", "last_applied_at": nil, "has_config": false, "deployment": nil, "deployment_error": "", "workload": nil, "chart_ready": false, "chart_failed": false, "chart_status": "", "lb_active": false, "controller_mode": string(k8s.UnknownMode), "detection_evidence": []string{}, "capabilities": capabilities(chart, k8s.UnknownMode), "availability": map[string]interface{}{"default_replicas": profile(chart).DefaultReplicas, "high_availability": profile(chart).SupportsHA, "node_placement": profile(chart).SupportsNodePlacement, "description": profile(chart).Description}}
		if cfg := configs[chart]; cfg != nil {
			item["values_content"] = cfg.ValuesContent
			item["enabled"] = cfg.Enabled
			item["apply_status"] = cfg.ApplyStatus
			item["apply_error"] = cfg.ApplyError
			item["last_applied_at"] = cfg.LastAppliedAt
			item["has_config"] = true
			item["saved_controller_mode"] = cfg.ControllerMode
		}
		if chart == "traefik" {
			item["traefik"] = traefikTimeoutPayload(item["values_content"].(string), nil)
		}
		detection, err := s.Adapter.DetectSystemComponent(ctx, ns, chart)
		if err != nil {
			item["deployment_error"] = err.Error()
			item["detection_error"] = err.Error()
		} else {
			item["controller_mode"] = string(detection.Mode)
			item["detection_evidence"] = detection.Evidence
			item["capabilities"] = capabilities(chart, detection.Mode)
			item["workload"] = workloadPayload(detection.Workload)
			item["chart_ready"] = detection.ChartReady
			item["chart_failed"] = detection.ChartFailed
			item["chart_status"] = detection.ChartStatus
			if detection.Mode == k8s.EmbeddedMode {
				item["lb_active"] = true
			}
			if detection.Deployment != nil {
				item["deployment"] = deploymentPayload(detection.Deployment)
				item["effective"] = true
				item["effective_detail"] = ""
				if cfg := configs[chart]; cfg != nil && cfg.ValuesContent != "" {
					item["effective"], item["effective_detail"] = effective(cfg.ValuesContent, detection.Deployment)
				}
				if chart == "traefik" {
					state := traefikTimeoutPayload(item["values_content"].(string), detection.Deployment)
					item["traefik"] = state
					if state["read_timeout"] != "" {
						item["effective"] = state["read_timeout_effective"]
						if state["read_timeout_effective"] == false {
							item["effective_detail"] = "入口请求读取超时与实际 Traefik 参数不一致"
						}
					}
				}
			}
		}
		result = append(result, item)
	}
	return result, nil
}

func traefikTimeoutPayload(values string, deployment *appsv1.Deployment) map[string]interface{} {
	desired := ConfiguredReadTimeout(values)
	if desired == "" {
		return map[string]interface{}{"read_timeout": "", "effective_read_timeout": "60s", "read_timeout_effective": true}
	}
	actual, effective := k8s.TraefikReadTimeout(deployment, desired)
	return map[string]interface{}{"read_timeout": desired, "effective_read_timeout": actual, "read_timeout_effective": effective}
}
func workloadPayload(w *k8s.ComponentWorkload) interface{} {
	if w == nil {
		return nil
	}
	return map[string]interface{}{"kind": w.Kind, "name": w.Name, "desired": w.Desired, "ready": w.Ready, "available": w.Available}
}
func deploymentPayload(d *appsv1.Deployment) map[string]interface{} {
	image := ""
	if len(d.Spec.Template.Spec.Containers) > 0 {
		image = d.Spec.Template.Spec.Containers[0].Image
	}
	return map[string]interface{}{"replicas": d.Status.Replicas, "ready_replicas": d.Status.ReadyReplicas, "available_replicas": d.Status.AvailableReplicas, "strategy": d.Spec.Strategy, "image": image, "node_selector": d.Spec.Template.Spec.NodeSelector, "fixed_node": d.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]}
}
func effective(values string, d *appsv1.Deployment) (bool, string) {
	var v map[string]interface{}
	if err := yaml.Unmarshal([]byte(values), &v); err != nil {
		return false, "values 配置解析失败"
	}
	issues := []string{}
	if raw, ok := v["replicas"]; ok {
		if n, ok := intValue(raw); ok && d.Spec.Replicas != nil && int32(n) != *d.Spec.Replicas {
			issues = append(issues, "replicas")
		}
	}
	rolling := d.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType && d.Spec.Strategy.RollingUpdate != nil
	actualUnavailable, actualSurge := "", ""
	if rolling {
		actualUnavailable = d.Spec.Strategy.RollingUpdate.MaxUnavailable.String()
		actualSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.String()
	}
	if mu, _ := rolloutValues(v); mu != nil && fmt.Sprint(mu) != actualUnavailable {
		issues = append(issues, "maxUnavailable")
	}
	if _, ms := rolloutValues(v); ms != nil && fmt.Sprint(ms) != actualSurge {
		issues = append(issues, "maxSurge")
	}
	if raw, ok := v["nodeSelector"].(map[string]interface{}); ok {
		expected := ""
		if value, found := raw[corev1.LabelHostname]; found {
			expected = fmt.Sprint(value)
		}
		if expected != d.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] {
			issues = append(issues, "nodeSelector")
		}
	}
	if len(issues) == 0 {
		return true, ""
	}
	return false, strings.Join(issues, "、") + " 与实际 Deployment 不一致"
}
func rolloutValues(v map[string]interface{}) (interface{}, interface{}) {
	mu, ms := v["maxUnavailable"], v["maxSurge"]
	if st, ok := v["deploymentStrategy"].(map[string]interface{}); ok {
		if ru, ok := st["rollingUpdate"].(map[string]interface{}); ok {
			if x, ok := ru["maxUnavailable"]; ok {
				mu = x
			}
			if x, ok := ru["maxSurge"]; ok {
				ms = x
			}
		}
	}
	return mu, ms
}
func Effective(values string, d *appsv1.Deployment) (bool, string) { return effective(values, d) }
func intValue(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}
