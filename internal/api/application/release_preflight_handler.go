package applicationapi

import (
	"fmt"
	"sort"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
)

type releasePreflightCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func (h *ApplicationHandler) PreflightRelease(c *gin.Context) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	var req releaseRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TemplateID == 0 {
		apiShared.BadRequest(c, "请选择上线模板")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	template, err := h.resources.GetApplicationDeploymentTemplate(applicationID, req.TemplateID)
	if err != nil || !template.Enabled {
		apiShared.ValidationError(c, "上线模板不存在或已停用")
		return
	}
	checks := make([]releasePreflightCheck, 0, 8)
	add := func(name string, err error, detail string) {
		if err != nil {
			checks = append(checks, releasePreflightCheck{Name: name, Detail: err.Error()})
		} else {
			checks = append(checks, releasePreflightCheck{Name: name, OK: true, Detail: detail})
		}
	}
	spec, err := h.releaseWorkflow().PrepareTemplateSpec(app, template, strings.TrimSpace(req.Version))
	if err != nil {
		add("模板与密钥", err, "")
		apiShared.Success(c, gin.H{"ready": false, "checks": checks})
		return
	}
	configKeys := make([]string, 0, len(spec.Config))
	disabledConfig := stringSet(spec.ConfigDisabled)
	for key := range spec.Config {
		if !disabledConfig[key] {
			configKeys = append(configKeys, key)
		}
	}
	sort.Strings(configKeys)
	secretKeys := make([]string, 0, len(spec.Secrets))
	missingSecrets := make([]string, 0)
	disabledSecrets := stringSet(spec.SecretsDisabled)
	for key, value := range spec.Secrets {
		if disabledSecrets[key] {
			continue
		}
		secretKeys = append(secretKeys, key)
		if value == "" {
			missingSecrets = append(missingSecrets, key)
		}
	}
	sort.Strings(secretKeys)
	sort.Strings(missingSecrets)
	add("配置键", nil, fmt.Sprintf("%d 项：%s", len(configKeys), strings.Join(configKeys, "、")))
	if len(missingSecrets) > 0 {
		add("密钥", fmt.Errorf("未配置：%s", strings.Join(missingSecrets, "、")), "")
	} else {
		add("密钥", nil, fmt.Sprintf("%d 项已配置（不显示值）：%s", len(secretKeys), strings.Join(secretKeys, "、")))
	}
	if issues := applicationservice.ValidateReleaseSpec(spec); len(issues) > 0 {
		add("模板字段", fmt.Errorf("%s", issues[0].Message), "")
	} else {
		ports := make([]string, 0, len(spec.Service.PortSpecs()))
		for _, port := range spec.Service.PortSpecs() {
			protocol := port.Protocol
			if protocol == "" {
				protocol = applicationservice.ServiceProtocolTCP
			}
			ports = append(ports, fmt.Sprintf("%d/%s", port.Port, protocol))
		}
		add("模板字段", nil, fmt.Sprintf("镜像 %s；Service %s；容器端口 %d", spec.Image, strings.Join(ports, "、"), spec.ContainerPort))
	}
	add("镜像引用", h.kubernetes.ValidateImageReference(c.Request.Context(), spec), spec.Image)
	add("命名空间、节点、PVC 与挂载", h.kubernetes.Preflight(c.Request.Context(), applicationservice.ApplicationContext{Namespace: app.Environment.Namespace, EnvironmentID: app.EnvironmentID, ApplicationName: app.Name}, spec), fmt.Sprintf("节点 %s；PVC %d 个；文件挂载 %d 个", nodeLabel(spec.NodeName), len(spec.Volumes), len(spec.FileMounts)))
	ready := true
	for _, check := range checks {
		ready = ready && check.OK
	}
	apiShared.Success(c, gin.H{"ready": ready, "checks": checks})
}

func nodeLabel(name string) string {
	if name == "" {
		return "自动调度"
	}
	return name
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
