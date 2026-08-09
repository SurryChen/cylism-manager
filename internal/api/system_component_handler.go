package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// systemChartWhitelist 只允许管理 K3s 内置 chart，防止误操作任意 CRD。
var systemChartWhitelist = map[string]string{
	"coredns":                "kube-system",
	"traefik":                "kube-system",
	"metrics-server":         "kube-system",
	"local-path-provisioner": "kube-system",
	"servicelb":              "kube-system",
}

type SystemComponentHandler struct {
	store *store.Store
}

func NewSystemComponentHandler(s *store.Store) *SystemComponentHandler {
	return &SystemComponentHandler{store: s}
}

type systemComponentUpdateRequest struct {
	ValuesContent string `json:"values_content"`
}

func (h *SystemComponentHandler) List(c *gin.Context) {
	if K8s == nil || K8s.DynamicClient == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Kubernetes 集群未连接")
		return
	}
	configs := map[string]*model.SystemComponentConfig{}
	rows, err := h.store.ListSystemComponentConfigs()
	if err == nil {
		for index := range rows {
			configs[rows[index].ChartName] = &rows[index]
		}
	}
	ctx := c.Request.Context()
	result := make([]gin.H, 0, len(systemChartWhitelist))
	for chart, namespace := range systemChartWhitelist {
		item := gin.H{
			"chart_name":      chart,
			"namespace":       namespace,
			"values_content":  "",
			"enabled":         false,
			"apply_status":    "",
			"apply_error":     "",
			"last_applied_at": nil,
			"has_config":      false,
			"deployment":      nil,
		}
		if config := configs[chart]; config != nil {
			item["values_content"] = config.ValuesContent
			item["enabled"] = config.Enabled
			item["apply_status"] = config.ApplyStatus
			item["apply_error"] = config.ApplyError
			item["last_applied_at"] = config.LastAppliedAt
			item["has_config"] = true
		}
		deployment, getErr := K8s.Clientset.AppsV1().Deployments(namespace).Get(ctx, chart, metav1.GetOptions{})
		if getErr == nil && deployment != nil {
			image := ""
			if len(deployment.Spec.Template.Spec.Containers) > 0 {
				image = deployment.Spec.Template.Spec.Containers[0].Image
			}
			item["deployment"] = gin.H{
				"replicas":           deployment.Status.Replicas,
				"ready_replicas":     deployment.Status.ReadyReplicas,
				"available_replicas": deployment.Status.AvailableReplicas,
				"strategy":           deployment.Spec.Strategy,
				"image":              image,
			}
		}
		result = append(result, item)
	}
	model.Success(c, result)
}

func (h *SystemComponentHandler) Update(c *gin.Context) {
	chart := strings.TrimSpace(c.Param("chart"))
	namespace, ok := systemChartWhitelist[chart]
	if !ok {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "不支持该系统组件")
		return
	}
	if K8s == nil || K8s.DynamicClient == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Kubernetes 集群未连接")
		return
	}
	var req systemComponentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ValuesContent) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "values 配置不能为空")
		return
	}
	var values map[string]any
	if err := yaml.Unmarshal([]byte(req.ValuesContent), &values); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "values 配置不是合法 YAML: "+err.Error())
		return
	}
	now := time.Now()
	config := &model.SystemComponentConfig{
		ChartName:     chart,
		Namespace:     namespace,
		ValuesContent: strings.TrimSpace(req.ValuesContent),
		Enabled:       true,
		LastAppliedAt: &now,
		CreatedBy:     getUserID(c),
	}
	if err := K8s.ApplyHelmChartConfig(c.Request.Context(), namespace, chart, config.ValuesContent); err != nil {
		config.ApplyStatus = "failed"
		config.ApplyError = err.Error()
		_ = h.store.UpsertSystemComponentConfig(config)
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "应用系统组件配置失败: "+err.Error())
		return
	}
	config.ApplyStatus = "succeeded"
	if err := h.store.UpsertSystemComponentConfig(config); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存系统组件配置失败")
		return
	}
	model.SuccessWithMessage(c, config, "系统组件配置已应用")
}

func (h *SystemComponentHandler) Revert(c *gin.Context) {
	chart := strings.TrimSpace(c.Param("chart"))
	namespace, ok := systemChartWhitelist[chart]
	if !ok {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "不支持该系统组件")
		return
	}
	if K8s == nil || K8s.DynamicClient == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Kubernetes 集群未连接")
		return
	}
	if err := K8s.DeleteHelmChartConfig(c.Request.Context(), namespace, chart); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "恢复系统组件默认配置失败: "+err.Error())
		return
	}
	_ = h.store.DeleteSystemComponentConfig(chart)
	model.SuccessWithMessage(c, nil, "已恢复系统组件默认配置")
}
