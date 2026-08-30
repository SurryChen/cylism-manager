package infrastructure

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
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

type clusterDNSStore interface {
	GetActiveClusterDNSPolicy() (*model.ClusterDNSPolicy, error)
	ListClusterDNSPolicies(limit int) ([]model.ClusterDNSPolicy, error)
	CreateClusterDNSPolicy(policy *model.ClusterDNSPolicy) error
}

type ClusterDNSHandler struct {
	store clusterDNSStore
	k8s   *k8sclient.Client
}

func NewClusterDNSHandler(s clusterDNSStore, client *k8sclient.Client) *ClusterDNSHandler {
	return &ClusterDNSHandler{store: s, k8s: client}
}

type clusterDNSPolicyRequest struct {
	Resolvers []string `json:"resolvers"`
}

func (h *ClusterDNSHandler) Status(c *gin.Context) {
	if h.k8s == nil || h.k8s.Clientset == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	configMap, err := h.k8s.Clientset.CoreV1().ConfigMaps(coreDNSNamespace).Get(c.Request.Context(), coreDNSConfigMap, metav1.GetOptions{})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取 CoreDNS 配置失败")
		return
	}
	policy, err := h.store.GetActiveClusterDNSPolicy()
	if err != nil && !apierrors.IsNotFound(err) && !strings.Contains(err.Error(), "record not found") {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取 DNS 策略失败")
		return
	}
	history, err := h.store.ListClusterDNSPolicies(20)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取 DNS 策略历史失败")
		return
	}
	pods, err := h.k8s.Clientset.CoreV1().Pods(coreDNSNamespace).List(c.Request.Context(), metav1.ListOptions{LabelSelector: "k8s-app=kube-dns"})
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, "读取 CoreDNS Pod 状态失败")
		return
	}
	podStatus := make([]gin.H, 0, len(pods.Items))
	for _, pod := range pods.Items {
		podStatus = append(podStatus, gin.H{"name": pod.Name, "node": pod.Spec.NodeName, "ip": pod.Status.PodIP, "ready": coreDNSPodReady(&pod)})
	}
	sort.Slice(podStatus, func(i, j int) bool { return podStatus[i]["name"].(string) < podStatus[j]["name"].(string) })
	model.Success(c, gin.H{
		"forwarding":    forwardTargets(configMap.Data["Corefile"]),
		"active_policy": policyPayload(policy),
		"history":       historyPayload(history),
		"pods":          podStatus,
	})
}

func (h *ClusterDNSHandler) Apply(c *gin.Context) {
	if h.k8s == nil || h.k8s.Clientset == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	var request clusterDNSPolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "DNS 策略无效")
		return
	}
	resolvers, err := normalizeDNSResolvers(request.Resolvers)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	policy, err := h.applyResolvers(c, resolvers)
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, policyPayload(policy), "集群 DNS 策略已应用，CoreDNS 将自动重载配置")
}

// Reset restores K3s's default resolver forwarding. The empty resolver list
// is persisted as an inherited policy so the UI can distinguish reset from
// an unavailable or unmanaged CoreDNS configuration.
func (h *ClusterDNSHandler) Reset(c *gin.Context) {
	if h.k8s == nil || h.k8s.Clientset == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	policy, err := h.applyResolvers(c, nil)
	if err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, err.Error())
		return
	}
	model.SuccessWithMessage(c, policyPayload(policy), "已恢复使用各节点宿主机 DNS")
}

func (h *ClusterDNSHandler) applyResolvers(c *gin.Context, resolvers []string) (*model.ClusterDNSPolicy, error) {
	configMaps := h.k8s.Clientset.CoreV1().ConfigMaps(coreDNSNamespace)
	configMap, err := configMaps.Get(c.Request.Context(), coreDNSConfigMap, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 CoreDNS 配置失败")
	}
	updated, err := replaceCoreDNSForward(configMap.Data["Corefile"], resolvers)
	if err != nil {
		return nil, err
	}
	previous := configMap.Data["Corefile"]
	configMap.Data["Corefile"] = updated
	if _, err := configMaps.Update(c.Request.Context(), configMap, metav1.UpdateOptions{}); err != nil {
		return nil, fmt.Errorf("更新 CoreDNS 配置失败")
	}
	encodedResolvers := resolvers
	if encodedResolvers == nil {
		encodedResolvers = []string{}
	}
	encoded, _ := json.Marshal(encodedResolvers)
	policy := &model.ClusterDNSPolicy{Resolvers: string(encoded), CreatedBy: apiShared.UserID(c)}
	if err := h.store.CreateClusterDNSPolicy(policy); err != nil {
		configMap.Data["Corefile"] = previous
		_, _ = configMaps.Update(c.Request.Context(), configMap, metav1.UpdateOptions{})
		return nil, fmt.Errorf("保存 DNS 策略历史失败")
	}
	return policy, nil
}

func (h *ClusterDNSHandler) Rollback(c *gin.Context) {
	revision := strings.TrimSpace(c.Param("revision"))
	history, err := h.store.ListClusterDNSPolicies(50)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取 DNS 策略历史失败")
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
			applied, applyErr := h.applyResolvers(c, nil)
			if applyErr != nil {
				model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, applyErr.Error())
				return
			}
			model.SuccessWithMessage(c, policyPayload(applied), "DNS 策略已回滚为宿主机 DNS，CoreDNS 将自动重载配置")
			return
		}
		resolvers, normalizeErr := normalizeDNSResolvers(resolvers)
		if normalizeErr != nil {
			break
		}
		applied, applyErr := h.applyResolvers(c, resolvers)
		if applyErr != nil {
			model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, applyErr.Error())
			return
		}
		model.SuccessWithMessage(c, policyPayload(applied), "DNS 策略已回滚，CoreDNS 将自动重载配置")
		return
	}
	model.Error(c, http.StatusNotFound, model.CodeNotFound, "DNS 策略版本不存在")
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
	matches := coreDNSForwardPattern.FindStringSubmatch(corefile)
	if len(matches) < 3 {
		return []string{}
	}
	return strings.Fields(strings.TrimSpace(matches[2]))
}

func policyPayload(policy *model.ClusterDNSPolicy) any {
	if policy == nil {
		return nil
	}
	var resolvers []string
	_ = json.Unmarshal([]byte(policy.Resolvers), &resolvers)
	return gin.H{"revision": policy.Revision, "resolvers": resolvers, "created_at": policy.CreatedAt}
}

func historyPayload(policies []model.ClusterDNSPolicy) []gin.H {
	result := make([]gin.H, 0, len(policies))
	for index := range policies {
		if payload, ok := policyPayload(&policies[index]).(gin.H); ok {
			result = append(result, payload)
		}
	}
	return result
}

func coreDNSPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
