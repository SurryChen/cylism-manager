package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const registryProxyNamespace = "kube-system"
const registryProxyName = "cylism-registry-proxy"

type RegistryProxyHandler struct{ store *store.Store }

type registryProxyRequest struct {
	NodeName             string `json:"node_name"`
	EndpointHost         string `json:"endpoint_host"`
	NodePort             int32  `json:"node_port"`
	CacheLimitGi         int32  `json:"cache_limit_gi"`
	CleanupIntervalHours int32  `json:"cleanup_interval_hours"`
}

func NewRegistryProxyHandler(s *store.Store) *RegistryProxyHandler {
	return &RegistryProxyHandler{store: s}
}

func (h *RegistryProxyHandler) Get(c *gin.Context) {
	proxy, err := h.store.GetRegistryProxy()
	if apierrors.IsNotFound(err) || err != nil && strings.Contains(err.Error(), "record not found") {
		model.Success(c, nil)
		return
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取镜像代理失败")
		return
	}
	h.refreshStatus(c.Request.Context(), proxy)
	model.Success(c, proxy)
}

func (h *RegistryProxyHandler) Deploy(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var req registryProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "代理配置无效")
		return
	}
	if err := validateRegistryProxyRequest(req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if _, err := K8s.Clientset.CoreV1().Nodes().Get(c.Request.Context(), req.NodeName, metav1.GetOptions{}); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "部署节点不存在或未加入集群")
		return
	}
	proxy, err := h.store.GetRegistryProxy()
	if err != nil {
		proxy = &model.RegistryProxy{CreatedBy: getUserID(c)}
	}
	proxy.NodeName, proxy.EndpointHost, proxy.NodePort = req.NodeName, req.EndpointHost, req.NodePort
	proxy.CacheLimitGi, proxy.CleanupIntervalHours = req.CacheLimitGi, req.CleanupIntervalHours
	proxy.Status, proxy.LastError = "deploying", ""
	if err := h.store.SaveRegistryProxy(proxy); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存代理配置失败")
		return
	}
	if err := h.apply(c.Request.Context(), proxy); err != nil {
		proxy.Status, proxy.LastError = "failed", err.Error()
		_ = h.store.SaveRegistryProxy(proxy)
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "部署镜像代理失败: "+err.Error())
		return
	}
	model.SuccessWithMessage(c, proxy, "镜像代理已提交部署，稍后可在节点镜像源中使用该地址")
}

func (h *RegistryProxyHandler) Cleanup(c *gin.Context) {
	proxy, err := h.store.GetRegistryProxy()
	if err != nil || K8s == nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像代理不存在或集群未连接")
		return
	}
	if err := h.clearCache(c.Request.Context(), proxy, "手动清理"); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	model.SuccessWithMessage(c, proxy, "代理 Pod 已重建，临时缓存正在清理")
}

func (h *RegistryProxyHandler) Reconcile() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if K8s == nil {
			continue
		}
		proxy, err := h.store.GetRegistryProxy()
		if err == nil {
			h.refreshStatus(context.Background(), proxy)
		}
	}
}

func (h *RegistryProxyHandler) refreshStatus(ctx context.Context, proxy *model.RegistryProxy) {
	if K8s == nil {
		return
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(ctx, registryProxyName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		proxy.Status, proxy.LastError = "missing", "代理 Deployment 不存在"
	} else if err != nil {
		proxy.Status, proxy.LastError = "failed", "读取代理状态失败: "+err.Error()
	} else if deployment.Status.AvailableReplicas > 0 {
		proxy.Status, proxy.LastError = "ready", ""
		if proxy.LastCleanupAt == nil {
			now := time.Now()
			proxy.LastCleanupAt = &now
		} else if time.Since(*proxy.LastCleanupAt) >= time.Duration(proxy.CleanupIntervalHours)*time.Hour {
			_ = h.clearCache(ctx, proxy, "定期清理")
		}
	} else {
		proxy.Status = "deploying"
	}
	now := time.Now()
	proxy.LastCheckedAt = &now
	_ = h.store.SaveRegistryProxy(proxy)
}

func (h *RegistryProxyHandler) clearCache(ctx context.Context, proxy *model.RegistryProxy, reason string) error {
	if err := K8s.Clientset.CoreV1().Pods(registryProxyNamespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + registryProxyName}); err != nil {
		return fmt.Errorf("%s失败: %w", reason, err)
	}
	now := time.Now()
	proxy.LastCleanupAt, proxy.Status, proxy.LastError = &now, "deploying", reason+"后等待新 Pod 就绪"
	return h.store.SaveRegistryProxy(proxy)
}

func (h *RegistryProxyHandler) apply(ctx context.Context, proxy *model.RegistryProxy) error {
	labels := map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "app.kubernetes.io/name": registryProxyName}
	cacheLimit := resource.MustParse(strconv.Itoa(int(proxy.CacheLimitGi)) + "Gi")
	replicas := int32(1)
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: registryProxyName, Namespace: registryProxyNamespace, Labels: labels}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: proxy.NodeName}, Volumes: []corev1.Volume{{Name: "cache", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &cacheLimit}}}}, Containers: []corev1.Container{{Name: "registry", Image: "registry:2.8", Ports: []corev1.ContainerPort{{ContainerPort: 5000}}, Env: []corev1.EnvVar{{Name: "REGISTRY_PROXY_REMOTEURL", Value: "https://registry-1.docker.io"}, {Name: "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", Value: "/var/lib/registry"}}, VolumeMounts: []corev1.VolumeMount{{Name: "cache", MountPath: "/var/lib/registry"}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi")}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/v2/", Port: intstr.FromInt(5000)}}, InitialDelaySeconds: 3, PeriodSeconds: 5}}}}}}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: registryProxyName, Namespace: registryProxyNamespace, Labels: labels}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: labels, Ports: []corev1.ServicePort{{Name: "registry", Port: 5000, TargetPort: intstr.FromInt(5000), NodePort: proxy.NodePort}}}}
	if current, err := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(ctx, registryProxyName, metav1.GetOptions{}); err == nil {
		deployment.ResourceVersion = current.ResourceVersion
		_, err = K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Update(ctx, deployment, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	} else if _, err := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		return err
	}
	if current, err := K8s.Clientset.CoreV1().Services(registryProxyNamespace).Get(ctx, registryProxyName, metav1.GetOptions{}); err == nil {
		service.ResourceVersion = current.ResourceVersion
		_, err = K8s.Clientset.CoreV1().Services(registryProxyNamespace).Update(ctx, service, metav1.UpdateOptions{})
		return err
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	_, err := K8s.Clientset.CoreV1().Services(registryProxyNamespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

func validateRegistryProxyRequest(req registryProxyRequest) error {
	if strings.TrimSpace(req.NodeName) == "" || !privateOrTailnetIP(strings.TrimSpace(req.EndpointHost)) {
		return fmt.Errorf("代理地址必须是节点间可访问的私网或 Tailscale IP")
	}
	if req.NodePort < 30000 || req.NodePort > 32767 {
		return fmt.Errorf("NodePort 必须在 30000 到 32767 之间")
	}
	if req.CacheLimitGi < 1 || req.CacheLimitGi > 100 {
		return fmt.Errorf("临时缓存上限必须在 1 到 100 Gi 之间")
	}
	if req.CleanupIntervalHours < 1 || req.CleanupIntervalHours > 168 {
		return fmt.Errorf("清理周期必须在 1 到 168 小时之间")
	}
	return nil
}

func privateOrTailnetIP(value string) bool {
	ip := net.ParseIP(value)
	if ip == nil || ip.IsLoopback() {
		return false
	}
	if ip.IsPrivate() {
		return true
	}
	_, tailnet, _ := net.ParseCIDR("100.64.0.0/10")
	return tailnet.Contains(ip)
}
