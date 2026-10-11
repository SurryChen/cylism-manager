package applicationapi

import (
	"strconv"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
)

type applicationPodInfo struct {
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	Phase       string    `json:"phase"`
	Reason      string    `json:"reason,omitempty"`
	Ready       bool      `json:"ready"`
	Node        string    `json:"node"`
	Restarts    int32     `json:"restarts"`
	CreatedAt   time.Time `json:"created_at"`
	Containers  []string  `json:"containers"`
	CPUMilli    *int64    `json:"cpu_millicores,omitempty"`
	MemoryBytes *int64    `json:"memory_bytes,omitempty"`
}

func (h *ApplicationHandler) applicationPods(c *gin.Context) ([]corev1.Pod, string, bool) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return nil, "", false
	}
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return nil, "", false
	}
	app, err := h.queries.GetApplication(id)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return nil, "", false
	}
	namespace := app.Environment.Namespace
	selector := applicationservice.ManagedByLabel + "=" + applicationservice.ManagedByValue + "," + applicationservice.ApplicationNameLabel + "=" + app.Name
	pods, err := h.kubernetes.ListRuntimePods(c.Request.Context(), namespace, selector)
	if err != nil {
		apiShared.K8sAPIError(c, "读取应用 Pod 失败: "+err.Error())
		return nil, "", false
	}
	managed := make([]corev1.Pod, 0, len(pods))
	for _, pod := range pods {
		if pod.Labels[applicationservice.ManagedByLabel] == applicationservice.ManagedByValue && pod.Labels[applicationservice.ApplicationNameLabel] == app.Name {
			managed = append(managed, pod)
		}
	}
	return managed, namespace, true
}

func (h *ApplicationHandler) ListApplicationPods(c *gin.Context) {
	pods, namespace, ok := h.applicationPods(c)
	if !ok {
		return
	}
	usage, metricsErr := h.kubernetes.ListRuntimePodMetrics(c.Request.Context(), namespace)
	items := make([]applicationPodInfo, 0, len(pods))
	for _, pod := range pods {
		item := applicationPodInfo{
			Name: pod.Name, Namespace: namespace, Phase: string(pod.Status.Phase), Node: pod.Spec.NodeName,
			Ready:     pod.Status.Phase == corev1.PodRunning && len(pod.Status.ContainerStatuses) > 0,
			CreatedAt: pod.CreationTimestamp.Time, Containers: make([]string, 0, len(pod.Spec.Containers)),
		}
		for _, container := range pod.Spec.Containers {
			item.Containers = append(item.Containers, container.Name)
		}
		for _, status := range pod.Status.ContainerStatuses {
			item.Restarts += status.RestartCount
			item.Ready = item.Ready && status.Ready
			if status.State.Waiting != nil {
				item.Reason = status.State.Waiting.Reason
			} else if status.State.Terminated != nil && item.Reason == "" {
				item.Reason = status.State.Terminated.Reason
			}
		}
		if metric, found := usage[pod.Name]; found {
			item.CPUMilli, item.MemoryBytes = &metric.CPUMilli, &metric.MemoryBytes
		}
		items = append(items, item)
	}
	apiShared.Success(c, gin.H{"pods": items, "metrics_available": metricsErr == nil})
}

func (h *ApplicationHandler) GetApplicationPodLogs(c *gin.Context) {
	pods, namespace, ok := h.applicationPods(c)
	if !ok {
		return
	}
	var selected *corev1.Pod
	for index := range pods {
		if pods[index].Name == c.Param("pod") {
			selected = &pods[index]
			break
		}
	}
	if selected == nil {
		apiShared.NotFound(c, "Pod 不属于当前应用")
		return
	}
	container := strings.TrimSpace(c.Query("container"))
	if container == "" && len(selected.Spec.Containers) == 1 {
		container = selected.Spec.Containers[0].Name
	}
	validContainer := false
	for _, item := range selected.Spec.Containers {
		validContainer = validContainer || item.Name == container
	}
	if !validContainer {
		apiShared.BadRequest(c, "请选择有效的容器")
		return
	}
	tail := int64(200)
	if value := c.Query("tail"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 || parsed > 500 {
			apiShared.BadRequest(c, "日志行数必须在 1 到 500 之间")
			return
		}
		tail = parsed
	}
	logs, err := h.kubernetes.ReadRuntimePodLogs(c.Request.Context(), namespace, selected.Name, container, tail, c.Query("previous") == "true")
	if err != nil {
		apiShared.K8sAPIError(c, "读取容器日志失败: "+err.Error())
		return
	}
	apiShared.Success(c, gin.H{"logs": logs})
}
