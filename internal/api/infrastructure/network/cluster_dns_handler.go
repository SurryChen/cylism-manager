package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	coreDNSNamespace = "kube-system"
	coreDNSConfigMap = "coredns"
)

var coreDNSForwardPattern = regexp.MustCompile(`(?m)^(\s*forward\s+\.\s+)([^\n{]+)(\{[^\n]*\})?\s*$`)

type ClusterDNSHandler struct {
	store          repository.ClusterDNSRepository
	k8s            ClusterDNSAdapter
	adapterFactory ClusterDNSAdapterFactory
}

// ClusterDNSAdapterFactory resolves the Kubernetes API for a logical
// cluster. The empty query parameter keeps the legacy single-cluster mode
// working while cluster detail pages opt into Agent access explicitly.
type ClusterDNSAdapterFactory func(context.Context, uint) (ClusterDNSAdapter, error)

// ClusterDNSAdapter is the minimal Kubernetes capability required by this
// handler. The concrete client is composed by bootstrap, keeping transport
// code independent from the Kubernetes SDK.
type ClusterDNSAdapter interface {
	KubernetesAvailable() bool
	GetCoreDNSConfig(context.Context) (*corev1.ConfigMap, error)
	UpdateCoreDNSConfig(context.Context, *corev1.ConfigMap) (*corev1.ConfigMap, error)
	ListCoreDNSPods(context.Context) (*corev1.PodList, error)
}

type clientClusterDNSAdapter struct{ client *k8sclient.Client }

// NewClusterDNSAdapter adapts the Kubernetes client at the composition root.
// Handlers should receive the returned narrow interface instead of a client.
func NewClusterDNSAdapter(client *k8sclient.Client) ClusterDNSAdapter {
	if client == nil {
		return nil
	}
	return clientClusterDNSAdapter{client: client}
}

func (a clientClusterDNSAdapter) KubernetesAvailable() bool {
	return a.client != nil && a.client.Clientset != nil
}
func (a clientClusterDNSAdapter) GetCoreDNSConfig(ctx context.Context) (*corev1.ConfigMap, error) {
	return a.client.Clientset.CoreV1().ConfigMaps(coreDNSNamespace).Get(ctx, coreDNSConfigMap, metav1.GetOptions{})
}
func (a clientClusterDNSAdapter) UpdateCoreDNSConfig(ctx context.Context, config *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return a.client.Clientset.CoreV1().ConfigMaps(coreDNSNamespace).Update(ctx, config, metav1.UpdateOptions{})
}
func (a clientClusterDNSAdapter) ListCoreDNSPods(ctx context.Context) (*corev1.PodList, error) {
	return a.client.Clientset.CoreV1().Pods(coreDNSNamespace).List(ctx, metav1.ListOptions{LabelSelector: "k8s-app=kube-dns"})
}

// NewClusterDNSHandlerWithAdapter receives the narrow Kubernetes port owned
// by Bootstrap and does not inspect concrete client types.
func NewClusterDNSHandlerWithAdapter(s repository.ClusterDNSRepository, adapter ClusterDNSAdapter) *ClusterDNSHandler {
	return &ClusterDNSHandler{store: s, k8s: adapter}
}

func (h *ClusterDNSHandler) WithAdapterFactory(factory ClusterDNSAdapterFactory) *ClusterDNSHandler {
	h.adapterFactory = factory
	return h
}

func (h *ClusterDNSHandler) adapterForContext(c *gin.Context) (ClusterDNSAdapter, error) {
	value := strings.TrimSpace(c.Query("cluster_id"))
	if value == "" {
		return h.k8s, nil
	}
	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil || id == 0 {
		return nil, fmt.Errorf("集群 ID 无效")
	}
	if h.adapterFactory == nil {
		return nil, fmt.Errorf("集群 Agent 访问未配置")
	}
	return h.adapterFactory(c.Request.Context(), uint(id))
}

func requestedClusterID(c *gin.Context) uint {
	value := strings.TrimSpace(c.Query("cluster_id"))
	if value == "" {
		return 0
	}
	id, _ := strconv.ParseUint(value, 10, 32)
	return uint(id)
}

func (h *ClusterDNSHandler) activePolicy(clusterID uint) (*model.ClusterDNSPolicy, error) {
	if clusterID != 0 {
		if repo, ok := h.store.(repository.ClusterScopedDNSRepository); ok {
			return repo.GetActiveClusterDNSPolicyForCluster(clusterID)
		}
	}
	return h.store.GetActiveClusterDNSPolicy()
}

func (h *ClusterDNSHandler) policies(clusterID uint, limit int) ([]model.ClusterDNSPolicy, error) {
	if clusterID != 0 {
		if repo, ok := h.store.(repository.ClusterScopedDNSRepository); ok {
			return repo.ListClusterDNSPoliciesForCluster(clusterID, limit)
		}
	}
	return h.store.ListClusterDNSPolicies(limit)
}

func (h *ClusterDNSHandler) createPolicy(policy *model.ClusterDNSPolicy, clusterID uint) error {
	if clusterID != 0 {
		if repo, ok := h.store.(repository.ClusterScopedDNSRepository); ok {
			return repo.CreateClusterDNSPolicyForCluster(policy, clusterID)
		}
	}
	return h.store.CreateClusterDNSPolicy(policy)
}

type clusterDNSPolicyRequest struct {
	Resolvers []string `json:"resolvers"`
}

func (h *ClusterDNSHandler) Status(c *gin.Context) {
	adapter, err := h.adapterForContext(c)
	if err != nil {
		apiShared.K8sAPIError(c, err.Error())
		return
	}
	if adapter == nil || !adapter.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	configMap, err := adapter.GetCoreDNSConfig(c.Request.Context())
	if err != nil {
		apiShared.K8sAPIError(c, "读取 CoreDNS 配置失败")
		return
	}
	clusterID := requestedClusterID(c)
	policy, err := h.activePolicy(clusterID)
	if err != nil && !apierrors.IsNotFound(err) && !strings.Contains(err.Error(), "record not found") {
		apiShared.DBError(c, "读取 DNS 策略失败")
		return
	}
	history, err := h.policies(clusterID, 20)
	if err != nil {
		apiShared.DBError(c, "读取 DNS 策略历史失败")
		return
	}
	pods, err := adapter.ListCoreDNSPods(c.Request.Context())
	if err != nil {
		apiShared.K8sAPIError(c, "读取 CoreDNS Pod 状态失败")
		return
	}
	podStatus := make([]gin.H, 0, len(pods.Items))
	for _, pod := range pods.Items {
		podStatus = append(podStatus, gin.H{"name": pod.Name, "node": pod.Spec.NodeName, "ip": pod.Status.PodIP, "ready": coreDNSPodReady(&pod)})
	}
	sort.Slice(podStatus, func(i, j int) bool { return podStatus[i]["name"].(string) < podStatus[j]["name"].(string) })
	apiShared.Success(c, gin.H{
		"forwarding":    forwardTargets(configMap.Data["Corefile"]),
		"active_policy": policyPayload(policy),
		"history":       historyPayload(history),
		"pods":          podStatus,
	})
}

func (h *ClusterDNSHandler) Apply(c *gin.Context) {
	adapter, err := h.adapterForContext(c)
	if err != nil {
		apiShared.K8sAPIError(c, err.Error())
		return
	}
	if adapter == nil || !adapter.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	var request clusterDNSPolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "DNS 策略无效")
		return
	}
	resolvers, err := normalizeDNSResolvers(request.Resolvers)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	policy, err := h.applyResolvers(c, adapter, resolvers)
	if err != nil {
		apiShared.K8sAPIError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, policyPayload(policy), "集群 DNS 策略已应用，CoreDNS 将自动重载配置")
}

// Reset restores K3s's default resolver forwarding. The empty resolver list
// is persisted as an inherited policy so the UI can distinguish reset from
// an unavailable or unmanaged CoreDNS configuration.
func (h *ClusterDNSHandler) Reset(c *gin.Context) {
	adapter, err := h.adapterForContext(c)
	if err != nil {
		apiShared.K8sAPIError(c, err.Error())
		return
	}
	if adapter == nil || !adapter.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	policy, err := h.applyResolvers(c, adapter, nil)
	if err != nil {
		apiShared.K8sAPIError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, policyPayload(policy), "已恢复使用各节点宿主机 DNS")
}

func (h *ClusterDNSHandler) applyResolvers(c *gin.Context, adapter ClusterDNSAdapter, resolvers []string) (*model.ClusterDNSPolicy, error) {
	configMap, err := adapter.GetCoreDNSConfig(c.Request.Context())
	if err != nil {
		return nil, fmt.Errorf("读取 CoreDNS 配置失败")
	}
	updated, err := replaceCoreDNSForward(configMap.Data["Corefile"], resolvers)
	if err != nil {
		return nil, err
	}
	previous := configMap.Data["Corefile"]
	configMap.Data["Corefile"] = updated
	if _, err := adapter.UpdateCoreDNSConfig(c.Request.Context(), configMap); err != nil {
		return nil, fmt.Errorf("更新 CoreDNS 配置失败")
	}
	encodedResolvers := resolvers
	if encodedResolvers == nil {
		encodedResolvers = []string{}
	}
	encoded, _ := json.Marshal(encodedResolvers)
	policy := &model.ClusterDNSPolicy{Resolvers: string(encoded), CreatedBy: apiShared.UserID(c)}
	if err := h.createPolicy(policy, requestedClusterID(c)); err != nil {
		configMap.Data["Corefile"] = previous
		_, _ = adapter.UpdateCoreDNSConfig(c.Request.Context(), configMap)
		return nil, fmt.Errorf("保存 DNS 策略历史失败")
	}
	return policy, nil
}

func (h *ClusterDNSHandler) Rollback(c *gin.Context) {
	adapter, err := h.adapterForContext(c)
	if err != nil {
		apiShared.K8sAPIError(c, err.Error())
		return
	}
	if adapter == nil || !adapter.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	revision := strings.TrimSpace(c.Param("revision"))
	history, err := h.policies(requestedClusterID(c), 50)
	if err != nil {
		apiShared.DBError(c, "读取 DNS 策略历史失败")
		return
	}
	for _, policy := range history {
		if fmt.Sprint(policy.Revision) != revision {
			continue
		}
		var resolvers []string
		if json.Unmarshal([]byte(policy.Resolvers), &resolvers) != nil {
			break
		}
		if len(resolvers) == 0 {
			applied, applyErr := h.applyResolvers(c, adapter, nil)
			if applyErr != nil {
				apiShared.K8sAPIError(c, applyErr.Error())
				return
			}
			apiShared.SuccessWithMessage(c, policyPayload(applied), "DNS 策略已回滚为宿主机 DNS，CoreDNS 将自动重载配置")
			return
		}
		resolvers, normalizeErr := normalizeDNSResolvers(resolvers)
		if normalizeErr != nil {
			break
		}
		applied, applyErr := h.applyResolvers(c, adapter, resolvers)
		if applyErr != nil {
			apiShared.K8sAPIError(c, applyErr.Error())
			return
		}
		apiShared.SuccessWithMessage(c, policyPayload(applied), "DNS 策略已回滚，CoreDNS 将自动重载配置")
		return
	}
	apiShared.NotFound(c, "DNS 策略版本不存在")
}

func normalizeDNSResolvers(raw []string) ([]string, error) {
	if len(raw) < 1 || len(raw) > 3 {
		return nil, fmt.Errorf("请配置 1 到 3 个 DNS 上游地址")
	}
	seen, result := map[string]bool{}, make([]string, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if net.ParseIP(value) == nil {
			return nil, fmt.Errorf("DNS 上游必须是 IP 地址")
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("请至少配置一个 DNS 上游地址")
	}
	return result, nil
}

func replaceCoreDNSForward(corefile string, resolvers []string) (string, error) {
	if !coreDNSForwardPattern.MatchString(corefile) {
		return "", fmt.Errorf("未找到可由平台管理的 CoreDNS forward . 指令")
	}
	targets := "/etc/resolv.conf"
	if len(resolvers) > 0 {
		targets = strings.Join(resolvers, " ")
	}
	return coreDNSForwardPattern.ReplaceAllStringFunc(corefile, func(line string) string {
		matches := coreDNSForwardPattern.FindStringSubmatch(line)
		return matches[1] + targets + matches[3]
	}), nil
}

func forwardTargets(corefile string) []string {
	return networkservice.ForwardTargets(corefile)
}

func policyPayload(policy *model.ClusterDNSPolicy) any {
	return networkservice.PolicyPayload(policy)
}

func historyPayload(policies []model.ClusterDNSPolicy) []gin.H {
	result := make([]gin.H, 0, len(policies))
	for index := range policies {
		if payload := networkservice.PolicyPayload(&policies[index]); payload != nil {
			result = append(result, gin.H(payload))
		}
	}
	return result
}

func coreDNSPodReady(pod *corev1.Pod) bool {
	return networkservice.CoreDNSPodReady(pod)
}
