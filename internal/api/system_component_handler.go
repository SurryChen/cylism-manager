package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

type staticDeploymentValues struct {
	Replicas           *int32                    `json:"replicas"`
	DeploymentStrategy *staticDeploymentStrategy `json:"deploymentStrategy"`
	NodeSelector       map[string]string         `json:"nodeSelector"`
	MaxUnavailable     *intstr.IntOrString       `json:"maxUnavailable"`
	MaxSurge           *intstr.IntOrString       `json:"maxSurge"`
}

type staticDeploymentStrategy struct {
	Type          string                         `json:"type"`
	RollingUpdate *staticDeploymentRollingUpdate `json:"rollingUpdate"`
}

type staticDeploymentRollingUpdate struct {
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable"`
	MaxSurge       *intstr.IntOrString `json:"maxSurge"`
}

// componentAvailabilityProfile describes the deliberately narrow set of
// platform-owned availability actions. A static Deployment is not inherently
// horizontally scalable, so controller mode must not decide this by itself.
type componentAvailabilityProfile struct {
	DefaultReplicas       int32
	SupportsHA            bool
	SupportsNodePlacement bool
	Description           string
}

var systemComponentAvailabilityProfiles = map[string]componentAvailabilityProfile{
	"coredns": {
		DefaultReplicas:       1,
		SupportsHA:            true,
		SupportsNodePlacement: true,
		Description:           "支持高可用；启用前需要至少两个可调度节点。",
	},
	"metrics-server": {
		DefaultReplicas: 1,
		Description:     "副本由 K3s 管理，平台不提供通用双副本基线。",
	},
	"local-path-provisioner": {
		DefaultReplicas: 1,
		Description:     "保持单个活动 provisioner，平台不提供通用双副本基线。",
	},
}

func componentAvailability(chart string) componentAvailabilityProfile {
	if profile, ok := systemComponentAvailabilityProfiles[chart]; ok {
		return profile
	}
	return componentAvailabilityProfile{Description: "未定义高可用 profile，副本策略由组件自身管理。"}
}

func componentCapabilities(chart string, mode k8s.ControllerMode) gin.H {
	profile := componentAvailability(chart)
	static := mode == k8s.StaticDeploymentMode
	return gin.H{
		"configure":       mode == k8s.HelmChartMode || mode == k8s.StaticDeploymentMode,
		"node_placement":  static && profile.SupportsNodePlacement,
		"rollout":         static,
		"replica_scaling": static && profile.SupportsHA,
		"safe_baseline":   static && profile.SupportsHA,
		"restore":         mode == k8s.HelmChartMode || mode == k8s.StaticDeploymentMode,
	}
}

func componentAvailabilityPayload(chart string) gin.H {
	profile := componentAvailability(chart)
	return gin.H{
		"default_replicas":  profile.DefaultReplicas,
		"high_availability": profile.SupportsHA,
		"node_placement":    profile.SupportsNodePlacement,
		"description":       profile.Description,
	}
}

func (h *SystemComponentHandler) List(c *gin.Context) {
	if K8s == nil || K8s.Clientset == nil {
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
			"chart_name":         chart,
			"namespace":          namespace,
			"values_content":     "",
			"enabled":            false,
			"apply_status":       "",
			"apply_error":        "",
			"last_applied_at":    nil,
			"has_config":         false,
			"deployment":         nil,
			"deployment_error":   "",
			"workload":           nil,
			"chart_ready":        false,
			"chart_failed":       false,
			"chart_status":       "",
			"lb_active":          false,
			"controller_mode":    string(k8s.UnknownMode),
			"detection_evidence": []string{},
			"capabilities":       componentCapabilities(chart, k8s.UnknownMode),
			"availability":       componentAvailabilityPayload(chart),
		}
		if config := configs[chart]; config != nil {
			item["values_content"] = config.ValuesContent
			item["enabled"] = config.Enabled
			item["apply_status"] = config.ApplyStatus
			item["apply_error"] = config.ApplyError
			item["last_applied_at"] = config.LastAppliedAt
			item["has_config"] = true
			item["saved_controller_mode"] = config.ControllerMode
		}
		detection, detectErr := K8s.DetectSystemComponent(ctx, namespace, chart)
		if detectErr != nil {
			item["deployment_error"] = detectErr.Error()
			item["detection_error"] = detectErr.Error()
		} else {
			item["controller_mode"] = string(detection.Mode)
			item["detection_evidence"] = detection.Evidence
			item["capabilities"] = componentCapabilities(chart, detection.Mode)
			item["workload"] = workloadPayload(detection.Workload)
			item["chart_ready"] = detection.ChartReady
			item["chart_failed"] = detection.ChartFailed
			item["chart_status"] = detection.ChartStatus
			if detection.Mode == k8s.EmbeddedMode {
				item["lb_active"] = true
			}
		}
		deployment := detection.Deployment
		if deployment == nil && detection.Workload == nil && detection.Mode != k8s.HelmChartMode && item["deployment_error"] == "" {
			item["deployment_error"] = fmt.Sprintf("deployments.apps %q/%q not found", namespace, chart)
		} else if deployment != nil {
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
				"node_selector":      deployment.Spec.Template.Spec.NodeSelector,
				"fixed_node":         deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname],
			}
			item["effective"] = true
			item["effective_detail"] = ""
			if config := configs[chart]; config != nil && config.ValuesContent != "" {
				item["effective"], item["effective_detail"] = systemComponentEffective(config.ValuesContent, deployment)
			}
		}
		result = append(result, item)
	}
	model.Success(c, result)
}

func workloadPayload(workload *k8s.ComponentWorkload) any {
	if workload == nil {
		return nil
	}
	return gin.H{
		"kind":      workload.Kind,
		"name":      workload.Name,
		"desired":   workload.Desired,
		"ready":     workload.Ready,
		"available": workload.Available,
	}
}

func parseStaticDeploymentConfig(valuesContent, component string) (k8s.StaticDeploymentConfig, error) {
	var values staticDeploymentValues
	if err := yaml.Unmarshal([]byte(valuesContent), &values); err != nil {
		return k8s.StaticDeploymentConfig{}, err
	}
	if values.Replicas == nil || *values.Replicas < 1 {
		return k8s.StaticDeploymentConfig{}, fmt.Errorf("%s 副本数必须至少为 1", component)
	}
	strategy := values.DeploymentStrategy
	if strategy != nil && strategy.Type != "" && strategy.Type != string(appsv1.RollingUpdateDeploymentStrategyType) {
		return k8s.StaticDeploymentConfig{}, fmt.Errorf("%s 仅支持 RollingUpdate 策略", component)
	}
	var maxUnavailable, maxSurge *intstr.IntOrString
	if strategy != nil && strategy.RollingUpdate != nil {
		maxUnavailable = strategy.RollingUpdate.MaxUnavailable
		maxSurge = strategy.RollingUpdate.MaxSurge
	}
	// Accept the old top-level fields so previously persisted configurations can
	// be repaired by the platform's reconciliation loop.
	if maxUnavailable == nil {
		maxUnavailable = values.MaxUnavailable
	}
	if maxSurge == nil {
		maxSurge = values.MaxSurge
	}
	if maxUnavailable == nil || maxSurge == nil {
		return k8s.StaticDeploymentConfig{}, fmt.Errorf("%s 滚动策略必须包含 maxUnavailable 和 maxSurge", component)
	}
	return k8s.StaticDeploymentConfig{
		Replicas:       *values.Replicas,
		MaxUnavailable: *maxUnavailable,
		MaxSurge:       *maxSurge,
		NodeName:       strings.TrimSpace(values.NodeSelector[corev1.LabelHostname]),
	}, nil
}

func validateStaticDeploymentNode(nodeName string) error {
	if nodeName == "" {
		return nil
	}
	node, err := K8s.GetNodeInfo(nodeName)
	if err != nil {
		return fmt.Errorf("部署节点不存在或未加入集群")
	}
	if !node.Ready || node.Evicted {
		return fmt.Errorf("部署节点未就绪或已禁止调度")
	}
	return nil
}

func (h *SystemComponentHandler) applyStaticDeployment(ctx *gin.Context, namespace, name, valuesContent string) error {
	config, err := parseStaticDeploymentConfig(valuesContent, name)
	if err != nil {
		return err
	}
	if err := validateStaticDeploymentNode(config.NodeName); err != nil {
		return err
	}
	profile := componentAvailability(name)
	var deployment *appsv1.Deployment
	if profile.SupportsHA {
		deployment, err = K8s.Clientset.AppsV1().Deployments(namespace).Get(ctx.Request.Context(), name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("读取 %s 当前副本数失败: %w", name, err)
		}
		current := profile.DefaultReplicas
		if deployment.Spec.Replicas != nil {
			current = *deployment.Spec.Replicas
		}
		if config.Replicas > current {
			if err := h.preflightHAIncrease(ctx, namespace, name, deployment, config); err != nil {
				return err
			}
		}
	}
	if !profile.SupportsHA {
		deployment, getErr := K8s.Clientset.AppsV1().Deployments(namespace).Get(ctx.Request.Context(), name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("读取 %s 当前副本数失败: %w", name, getErr)
		}
		current := profile.DefaultReplicas
		if deployment.Spec.Replicas != nil {
			current = *deployment.Spec.Replicas
		}
		if config.Replicas != current {
			return fmt.Errorf("%s 副本由 K3s/组件 profile 管理，平台不支持修改为 %d 副本", name, config.Replicas)
		}
	}
	if config.NodeName != "" && !profile.SupportsNodePlacement {
		return fmt.Errorf("%s 不支持通过平台固定部署节点", name)
	}
	// Remove stale configs created by older Manager versions. A static component
	// is controlled by its Deployment, so retaining a HelmChartConfig would make
	// a future control-source change ambiguous.
	if K8s.DynamicClient != nil {
		if err := K8s.DeleteHelmChartConfig(ctx.Request.Context(), namespace, name); err != nil {
			return err
		}
	}
	return K8s.ApplyStaticDeploymentConfig(ctx.Request.Context(), namespace, name, config)
}

// preflightHAIncrease keeps the narrow CoreDNS HA baseline from turning a
// healthy singleton into two replicas that are both unschedulable, unhealthy,
// or pinned to one node. Kubernetes remains the final scheduler authority.
func (h *SystemComponentHandler) preflightHAIncrease(ctx *gin.Context, namespace, name string, deployment *appsv1.Deployment, config k8s.StaticDeploymentConfig) error {
	if config.Replicas < 2 {
		return nil
	}
	if config.NodeName != "" {
		return fmt.Errorf("%s 高可用副本不能固定在单个节点，请选择自动调度", name)
	}
	if deployment == nil || deployment.Status.ReadyReplicas < 1 || deployment.Status.AvailableReplicas < 1 {
		return fmt.Errorf("%s 当前未健康，不允许在故障状态下增加副本", name)
	}
	nodes, err := K8s.Clientset.CoreV1().Nodes().List(ctx.Request.Context(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("读取节点预检状态失败: %w", err)
	}
	candidates := 0
	for index := range nodes.Items {
		node := &nodes.Items[index]
		if node.Spec.Unschedulable || !systemComponentNodeReady(node) || node.Status.Allocatable.Cpu().IsZero() || node.Status.Allocatable.Memory().IsZero() {
			continue
		}
		candidates++
	}
	if candidates < 2 {
		return fmt.Errorf("%s 高可用需要至少 2 个 Ready、可调度且具备 CPU/内存可分配的节点，当前仅 %d 个", name, candidates)
	}
	pods, err := K8s.Clientset.CoreV1().Pods(namespace).List(ctx.Request.Context(), metav1.ListOptions{LabelSelector: "k8s-app=" + name})
	if err != nil {
		return fmt.Errorf("读取 %s Pod 预检状态失败: %w", name, err)
	}
	for index := range pods.Items {
		for _, status := range pods.Items[index].Status.ContainerStatuses {
			if status.State.Waiting == nil {
				continue
			}
			if status.State.Waiting.Reason == "ErrImagePull" || status.State.Waiting.Reason == "ImagePullBackOff" || status.State.Waiting.Reason == "InvalidImageName" {
				return fmt.Errorf("%s 存在镜像拉取失败，完成镜像源诊断后再增加副本", name)
			}
		}
	}
	return nil
}

func systemComponentNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// systemComponentEffective 对比平台保存的期望值与 Deployment 实际生效值，
// 返回配置是否真正生效。Helm 只渲染 chart 模板支持的 value，不支持的会被静默忽略。
func systemComponentEffective(valuesContent string, deployment *appsv1.Deployment) (bool, string) {
	var values map[string]any
	if err := yaml.Unmarshal([]byte(valuesContent), &values); err != nil {
		return false, "values 配置解析失败"
	}
	var issues []string
	if raw, ok := values["replicas"]; ok {
		if expected, ok := intValue(raw); ok && expected > 0 {
			actual := int32(0)
			if deployment.Spec.Replicas != nil {
				actual = *deployment.Spec.Replicas
			}
			if int32(expected) != actual {
				issues = append(issues, "replicas")
			}
		}
	}
	rolling := deployment.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType && deployment.Spec.Strategy.RollingUpdate != nil
	actualUnavailable := ""
	actualSurge := ""
	if rolling {
		actualUnavailable = deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.String()
		actualSurge = deployment.Spec.Strategy.RollingUpdate.MaxSurge.String()
	}
	maxUnavailable, maxSurge := rolloutValues(values)
	if raw := maxUnavailable; raw != nil {
		if fmt.Sprint(raw) != actualUnavailable {
			issues = append(issues, "maxUnavailable")
		}
	}
	if raw := maxSurge; raw != nil {
		if fmt.Sprint(raw) != actualSurge {
			issues = append(issues, "maxSurge")
		}
	}
	if raw, ok := values["nodeSelector"].(map[string]any); ok {
		expected := ""
		if value, configured := raw[corev1.LabelHostname]; configured {
			expected = fmt.Sprint(value)
		}
		if expected != deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] {
			issues = append(issues, "nodeSelector")
		}
	}
	if len(issues) == 0 {
		return true, ""
	}
	return false, strings.Join(issues, "、") + " 与实际 Deployment 不一致"
}

func rolloutValues(values map[string]any) (maxUnavailable, maxSurge any) {
	maxUnavailable = values["maxUnavailable"]
	maxSurge = values["maxSurge"]
	strategy, ok := values["deploymentStrategy"].(map[string]any)
	if !ok {
		return maxUnavailable, maxSurge
	}
	rollingUpdate, ok := strategy["rollingUpdate"].(map[string]any)
	if !ok {
		return maxUnavailable, maxSurge
	}
	if value, found := rollingUpdate["maxUnavailable"]; found {
		maxUnavailable = value
	}
	if value, found := rollingUpdate["maxSurge"]; found {
		maxSurge = value
	}
	return maxUnavailable, maxSurge
}

func intValue(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	}
	return 0, false
}

func (h *SystemComponentHandler) Update(c *gin.Context) {
	chart := strings.TrimSpace(c.Param("chart"))
	namespace, ok := systemChartWhitelist[chart]
	if !ok {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "不支持该系统组件")
		return
	}
	if K8s == nil || K8s.Clientset == nil {
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
	detection, detectErr := K8s.DetectSystemComponent(c.Request.Context(), namespace, chart)
	if detectErr != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "检测系统组件控制源失败: "+detectErr.Error())
		return
	}
	if detection.Mode == k8s.EmbeddedMode {
		model.Error(c, http.StatusConflict, model.CodeValidationFail, "该系统组件由 K3s 内置控制器管理，不支持通过平台修改")
		return
	}
	if detection.Mode == k8s.UnknownMode {
		model.Error(c, http.StatusConflict, model.CodeValidationFail, "无法识别该系统组件的控制源，已拒绝写入")
		return
	}
	now := time.Now()
	config := &model.SystemComponentConfig{
		ChartName:      chart,
		Namespace:      namespace,
		ControllerMode: string(detection.Mode),
		ValuesContent:  strings.TrimSpace(req.ValuesContent),
		Enabled:        true,
		LastAppliedAt:  &now,
		CreatedBy:      getUserID(c),
	}
	var applyErr error
	switch detection.Mode {
	case k8s.HelmChartMode:
		if K8s.DynamicClient == nil {
			applyErr = fmt.Errorf("Kubernetes 动态客户端未初始化")
		} else {
			applyErr = K8s.ApplyHelmChartConfig(c.Request.Context(), namespace, chart, config.ValuesContent)
		}
	case k8s.StaticDeploymentMode:
		applyErr = h.applyStaticDeployment(c, namespace, chart, config.ValuesContent)
	default:
		applyErr = fmt.Errorf("不支持的系统组件控制模式: %s", detection.Mode)
	}
	if applyErr != nil {
		config.ApplyStatus = "failed"
		config.ApplyError = applyErr.Error()
		_ = h.store.UpsertSystemComponentConfig(config)
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "应用系统组件配置失败: "+applyErr.Error())
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
	if K8s == nil || K8s.Clientset == nil {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Kubernetes 集群未连接")
		return
	}
	detection, detectErr := K8s.DetectSystemComponent(c.Request.Context(), namespace, chart)
	if detectErr != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "检测系统组件控制源失败: "+detectErr.Error())
		return
	}
	var revertErr error
	switch detection.Mode {
	case k8s.HelmChartMode:
		if K8s.DynamicClient == nil {
			revertErr = fmt.Errorf("Kubernetes 动态客户端未初始化")
		} else {
			revertErr = K8s.DeleteHelmChartConfig(c.Request.Context(), namespace, chart)
		}
	case k8s.StaticDeploymentMode:
		if K8s.DynamicClient != nil {
			revertErr = K8s.DeleteHelmChartConfig(c.Request.Context(), namespace, chart)
		}
		if revertErr == nil {
			revertErr = K8s.RestoreStaticDeploymentDefaults(c.Request.Context(), namespace, chart)
		}
	case k8s.EmbeddedMode, k8s.UnknownMode:
		model.Error(c, http.StatusConflict, model.CodeValidationFail, "该系统组件当前不支持恢复默认配置")
		return
	}
	if revertErr != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "恢复系统组件默认配置失败: "+revertErr.Error())
		return
	}
	_ = h.store.DeleteSystemComponentConfig(chart)
	model.SuccessWithMessage(c, nil, "已恢复系统组件默认配置")
}

// Reconcile re-applies stored static Deployment fields after a K3s manifest
// re-render. It re-detects every component first and stops if its control source
// changed, avoiding a fight with helm-controller or a user-installed workload.
func (h *SystemComponentHandler) Reconcile() {
	for {
		h.reconcileOnce()
		time.Sleep(5 * time.Minute)
	}
}

func (h *SystemComponentHandler) reconcileOnce() {
	if K8s == nil || K8s.Clientset == nil {
		return
	}
	configs, err := h.store.ListSystemComponentConfigs()
	if err != nil {
		return
	}
	for index := range configs {
		config := &configs[index]
		if !config.Enabled {
			continue
		}
		detection, detectErr := K8s.DetectSystemComponent(K8s.Ctx(), config.Namespace, config.ChartName)
		if detectErr != nil {
			config.ApplyStatus, config.ApplyError = "failed", detectErr.Error()
		} else if config.ControllerMode != "" && config.ControllerMode != string(detection.Mode) {
			config.ApplyStatus = "failed"
			config.ApplyError = "组件控制源已变更为 " + string(detection.Mode) + "，已停止自动重放"
		} else if detection.Mode == k8s.StaticDeploymentMode {
			parsed, parseErr := parseStaticDeploymentConfig(config.ValuesContent, config.ChartName)
			if parseErr == nil {
				parseErr = validateStaticDeploymentNode(parsed.NodeName)
			}
			if parseErr == nil {
				parseErr = K8s.ApplyStaticDeploymentConfig(K8s.Ctx(), config.Namespace, config.ChartName, parsed)
			}
			if parseErr != nil {
				config.ApplyStatus, config.ApplyError = "failed", parseErr.Error()
			} else {
				now := time.Now()
				config.LastAppliedAt = &now
				config.ApplyStatus, config.ApplyError = "succeeded", ""
			}
		}
		if detectErr == nil && (config.ControllerMode == "" || config.ControllerMode == string(detection.Mode)) {
			config.ControllerMode = string(detection.Mode)
		}
		_ = h.store.UpsertSystemComponentConfig(config)
	}
}
