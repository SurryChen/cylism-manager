package system_component

import (
	"fmt"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"
)

const (
	webReadTimeoutArg       = "--entryPoints.web.transport.respondingTimeouts.readTimeout="
	webSecureReadTimeoutArg = "--entryPoints.websecure.transport.respondingTimeouts.readTimeout="
)

type staticValues struct {
	Replicas           *int32              `json:"replicas"`
	DeploymentStrategy *strategy           `json:"deploymentStrategy"`
	NodeSelector       map[string]string   `json:"nodeSelector"`
	MaxUnavailable     *intstr.IntOrString `json:"maxUnavailable"`
	MaxSurge           *intstr.IntOrString `json:"maxSurge"`
}
type strategy struct {
	Type          string   `json:"type"`
	RollingUpdate *rolling `json:"rollingUpdate"`
}
type rolling struct {
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable"`
	MaxSurge       *intstr.IntOrString `json:"maxSurge"`
}

func ParseStaticDeploymentConfig(valuesContent, component string) (k8s.StaticDeploymentConfig, error) {
	var values staticValues
	if err := yaml.Unmarshal([]byte(valuesContent), &values); err != nil {
		return k8s.StaticDeploymentConfig{}, err
	}
	if values.Replicas == nil || *values.Replicas < 1 {
		return k8s.StaticDeploymentConfig{}, fmt.Errorf("%s 副本数必须至少为 1", component)
	}
	if values.DeploymentStrategy != nil && values.DeploymentStrategy.Type != "" && values.DeploymentStrategy.Type != string(appsv1.RollingUpdateDeploymentStrategyType) {
		return k8s.StaticDeploymentConfig{}, fmt.Errorf("%s 仅支持 RollingUpdate 策略", component)
	}
	var unavailable, surge *intstr.IntOrString
	if values.DeploymentStrategy != nil && values.DeploymentStrategy.RollingUpdate != nil {
		unavailable = values.DeploymentStrategy.RollingUpdate.MaxUnavailable
		surge = values.DeploymentStrategy.RollingUpdate.MaxSurge
	}
	if unavailable == nil {
		unavailable = values.MaxUnavailable
	}
	if surge == nil {
		surge = values.MaxSurge
	}
	if unavailable == nil || surge == nil {
		return k8s.StaticDeploymentConfig{}, fmt.Errorf("%s 滚动策略必须包含 maxUnavailable 和 maxSurge", component)
	}
	return k8s.StaticDeploymentConfig{Replicas: *values.Replicas, MaxUnavailable: *unavailable, MaxSurge: *surge, NodeName: strings.TrimSpace(values.NodeSelector[corev1.LabelHostname])}, nil
}

func RenderTraefikReadTimeout(valuesContent, timeout string) (string, error) {
	values := map[string]any{}
	if err := yaml.Unmarshal([]byte(valuesContent), &values); err != nil {
		return "", err
	}
	args := []string{}
	if raw, ok := values["additionalArguments"]; ok {
		list, ok := raw.([]any)
		if !ok {
			return "", fmt.Errorf("additionalArguments 必须是字符串列表")
		}
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("additionalArguments 必须是字符串列表")
			}
			if strings.HasPrefix(s, webReadTimeoutArg) || strings.HasPrefix(s, webSecureReadTimeoutArg) {
				continue
			}
			args = append(args, s)
		}
	}
	if timeout != "" {
		args = append(args, webReadTimeoutArg+timeout, webSecureReadTimeoutArg+timeout)
	}
	if len(args) == 0 {
		delete(values, "additionalArguments")
	} else {
		values["additionalArguments"] = args
	}
	out, err := yaml.Marshal(values)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
