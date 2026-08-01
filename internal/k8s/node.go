package k8s

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const mirrorPodAnnotation = "kubernetes.io/config.mirror"
const nodeDrainAnnotation = "cylism.io/drained-at"

const (
	nodeFailureThreshold = 5 * time.Minute

	NodeHealthReady    = "ready"
	NodeHealthNotReady = "not_ready"
	NodeHealthFailed   = "failed"
)

// NodeInfo is the node summary shown in the cluster console.
type NodeInfo struct {
	Name            string `json:"name"`
	Ready           bool   `json:"ready"`
	Evicted         bool   `json:"evicted"`
	Roles           string `json:"roles"`
	Version         string `json:"version"`
	InternalIP      string `json:"internal_ip"`
	OS              string `json:"os"`
	CPUCores        int64  `json:"cpu_cores"`
	MemoryMB        int64  `json:"memory_mb"`
	CreatedAt       string `json:"created_at"`
	HealthState     string `json:"health_state"`
	HealthReason    string `json:"health_reason,omitempty"`
	LastHeartbeatAt string `json:"last_heartbeat_at,omitempty"`
}

// NodeLabels is the label state exposed by the cluster node management API.
type NodeLabels struct {
	Name          string            `json:"name"`
	Labels        map[string]string `json:"labels"`
	ProtectedKeys []string          `json:"protected_keys"`
}

// DrainPod describes a Pod affected by a drain preflight or execution.
type DrainPod struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	OwnerKind string `json:"owner_kind,omitempty"`
	Reason    string `json:"reason"`
}

// DrainPlan separates Pods that can be evicted from the ones requiring action.
type DrainPlan struct {
	NodeName                     string     `json:"node_name"`
	AlreadyCordoned              bool       `json:"already_cordoned"`
	Evictable                    []DrainPod `json:"evictable"`
	RequiresEmptyDirConfirmation []DrainPod `json:"requires_empty_dir_confirmation"`
	Blocked                      []DrainPod `json:"blocked"`
	Skipped                      []DrainPod `json:"skipped"`
}

// DrainOptions controls explicit acceptance of emptyDir data loss.
type DrainOptions struct {
	DeleteEmptyDirData bool `json:"delete_empty_dir_data"`
}

// ForceDrainOptions requires explicit acknowledgement before bypassing PDBs on
// a worker that Kubernetes has already classified as failed.
type ForceDrainOptions struct {
	DeleteEmptyDirData bool   `json:"delete_empty_dir_data"`
	AcknowledgeRisk    bool   `json:"acknowledge_risk"`
	ConfirmNodeName    string `json:"confirm_node_name"`
}

// DrainResult reports each eviction request. Pending entries were protected by a PDB
// or failed to be submitted and are deliberately left running on the source node.
type DrainResult struct {
	Plan    *DrainPlan `json:"plan"`
	Evicted []DrainPod `json:"evicted"`
	Pending []DrainPod `json:"pending"`
	Forced  bool       `json:"forced,omitempty"`
	Deleted []DrainPod `json:"deleted,omitempty"`
	Failed  []DrainPod `json:"failed,omitempty"`
	Blocked []DrainPod `json:"blocked,omitempty"`
	Skipped []DrainPod `json:"skipped,omitempty"`
}

// NodeRemovalCheck explains whether deleting the Kubernetes Node object is safe.
type NodeRemovalCheck struct {
	NodeName  string     `json:"node_name"`
	CanRemove bool       `json:"can_remove"`
	Blockers  []DrainPod `json:"blockers"`
}

func (c *Client) ListNodeInfos() ([]NodeInfo, error) {
	nodes, err := c.Clientset.CoreV1().Nodes().List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	result := make([]NodeInfo, 0, len(nodes.Items))
	for index := range nodes.Items {
		result = append(result, nodeToInfo(&nodes.Items[index]))
	}
	return result, nil
}

func (c *Client) GetNodeInfo(name string) (*NodeInfo, error) {
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get node %s: %w", name, err)
	}
	info := nodeToInfo(node)
	return &info, nil
}

func (c *Client) GetNodeLabels(name string) (*NodeLabels, error) {
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get node %s labels: %w", name, err)
	}
	return nodeLabels(node), nil
}

// UpdateNodeLabels applies only validated custom labels. Kubernetes and K3s
// managed label prefixes are deliberately read-only in this control plane.
func (c *Client) UpdateNodeLabels(name string, set map[string]string, remove []string) (*NodeLabels, error) {
	if err := validateNodeLabelUpdate(set, remove); err != nil {
		return nil, err
	}
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get node %s labels: %w", name, err)
	}
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	for key, value := range set {
		node.Labels[key] = value
	}
	for _, key := range remove {
		delete(node.Labels, key)
	}
	updated, err := c.Clientset.CoreV1().Nodes().Update(c.Ctx(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update node %s labels: %w", name, err)
	}
	return nodeLabels(updated), nil
}

func nodeLabels(node *corev1.Node) *NodeLabels {
	labels := make(map[string]string, len(node.Labels))
	protected := make([]string, 0)
	for key, value := range node.Labels {
		labels[key] = value
		if isProtectedNodeLabel(key) {
			protected = append(protected, key)
		}
	}
	sort.Strings(protected)
	return &NodeLabels{Name: node.Name, Labels: labels, ProtectedKeys: protected}
}

func validateNodeLabelUpdate(set map[string]string, remove []string) error {
	for key, value := range set {
		if err := validateMutableNodeLabel(key, value); err != nil {
			return err
		}
	}
	for _, key := range remove {
		if err := validateMutableNodeLabel(key, ""); err != nil {
			return err
		}
		if _, alsoSet := set[key]; alsoSet {
			return fmt.Errorf("标签 %q 不能同时设置和删除", key)
		}
	}
	return nil
}

func validateMutableNodeLabel(key, value string) error {
	if isProtectedNodeLabel(key) {
		return fmt.Errorf("系统标签 %q 由 Kubernetes 或 K3s 管理，不能修改", key)
	}
	if issues := validation.IsQualifiedName(key); len(issues) > 0 {
		return fmt.Errorf("标签键 %q 无效: %s", key, strings.Join(issues, "; "))
	}
	if issues := validation.IsValidLabelValue(value); len(issues) > 0 {
		return fmt.Errorf("标签 %q 的值无效: %s", key, strings.Join(issues, "; "))
	}
	return nil
}

func isProtectedNodeLabel(key string) bool {
	return key == corev1.LabelHostname || strings.HasPrefix(key, "kubernetes.io/") || strings.HasPrefix(key, "node.kubernetes.io/") || strings.HasPrefix(key, "k3s.io/") || strings.HasPrefix(key, "node-role.kubernetes.io/") || strings.HasPrefix(key, "beta.kubernetes.io/")
}

// DrainPlan checks the node without modifying Pods. It never treats DaemonSet,
// mirror/static, terminal, or unmanaged Pods as ordinary eviction candidates.
func (c *Client) DrainPlan(name string) (*DrainPlan, error) {
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取节点失败: %w", err)
	}
	pods, err := c.Clientset.CoreV1().Pods("").List(c.Ctx(), metav1.ListOptions{FieldSelector: "spec.nodeName=" + name})
	if err != nil {
		return nil, fmt.Errorf("读取节点 Pod 失败: %w", err)
	}
	plan := &DrainPlan{NodeName: name, AlreadyCordoned: node.Spec.Unschedulable}
	for index := range pods.Items {
		pod := &pods.Items[index]
		item := drainPod(pod)
		switch {
		case pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed:
			item.Reason = "已完成的 Pod"
			plan.Skipped = append(plan.Skipped, item)
		case pod.Annotations[mirrorPodAnnotation] != "":
			item.Reason = "静态 Pod 由节点上的 K3s 管理"
			plan.Skipped = append(plan.Skipped, item)
		case isDaemonSetPod(pod):
			item.Reason = "DaemonSet Pod 不参与 drain"
			plan.Skipped = append(plan.Skipped, item)
		case item.OwnerKind == "":
			item.Reason = "未受控制器管理的 Pod 无法自动重建"
			plan.Blocked = append(plan.Blocked, item)
		case podUsesEmptyDir(pod):
			item.Reason = "包含 emptyDir，本地临时数据会丢失"
			plan.RequiresEmptyDirConfirmation = append(plan.RequiresEmptyDirConfirmation, item)
		default:
			item.Reason = "将通过 Eviction API 迁移"
			plan.Evictable = append(plan.Evictable, item)
		}
	}
	return plan, nil
}

// DrainNode cordons the node and submits PDB-aware eviction requests. It never
// directly deletes Pods, so a PodDisruptionBudget can defer an unsafe disruption.
func (c *Client) DrainNode(name string, options DrainOptions) (*DrainResult, error) {
	plan, err := c.DrainPlan(name)
	if err != nil {
		return nil, err
	}
	if len(plan.Blocked) > 0 {
		return &DrainResult{Plan: plan}, fmt.Errorf("存在未受控制器管理的 Pod，不能自动驱逐")
	}
	if len(plan.RequiresEmptyDirConfirmation) > 0 && !options.DeleteEmptyDirData {
		return &DrainResult{Plan: plan}, fmt.Errorf("存在包含 emptyDir 的 Pod，需要确认允许丢弃本地临时数据")
	}

	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取节点失败: %w", err)
	}
	if !node.Spec.Unschedulable {
		node.Spec.Unschedulable = true
	}
	markNodeDrained(node)
	if _, err := c.Clientset.CoreV1().Nodes().Update(c.Ctx(), node, metav1.UpdateOptions{}); err != nil {
		return nil, fmt.Errorf("标记节点不可调度失败: %w", err)
	}

	candidates := append(append([]DrainPod{}, plan.Evictable...), plan.RequiresEmptyDirConfirmation...)
	result := &DrainResult{Plan: plan}
	for _, item := range candidates {
		err := c.Clientset.CoreV1().Pods(item.Namespace).EvictV1(c.Ctx(), &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: item.Name, Namespace: item.Namespace}})
		if err == nil {
			result.Evicted = append(result.Evicted, item)
			continue
		}
		if isPDBViolation(err) {
			item.Reason = "PodDisruptionBudget 暂不允许驱逐: " + strings.TrimSpace(err.Error())
		} else {
			item.Reason = "提交驱逐请求失败: " + strings.TrimSpace(err.Error())
		}
		result.Pending = append(result.Pending, item)
	}
	return result, nil
}

// ForceDrainNode performs failure recovery for a confirmed failed worker. It
// directly deletes only controller-managed Pods, so Kubernetes recreates them
// on healthy nodes without waiting for a PDB that cannot be satisfied by a
// permanently unavailable source node.
func (c *Client) ForceDrainNode(name string, options ForceDrainOptions) (*DrainResult, error) {
	if !options.AcknowledgeRisk || options.ConfirmNodeName != name {
		return nil, fmt.Errorf("必须确认绕过 PodDisruptionBudget 并输入目标节点名")
	}
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取节点失败: %w", err)
	}
	if isControlPlaneNode(node) {
		return nil, fmt.Errorf("control-plane 节点不支持强制驱逐")
	}
	if nodeHealth(node, time.Now()).State != NodeHealthFailed {
		return nil, fmt.Errorf("节点尚未被判定为故障，不能绕过 PodDisruptionBudget")
	}
	plan, err := c.DrainPlan(name)
	if err != nil {
		return nil, err
	}
	result := &DrainResult{Plan: plan, Forced: true, Blocked: plan.Blocked, Skipped: plan.Skipped}
	if len(plan.RequiresEmptyDirConfirmation) > 0 && !options.DeleteEmptyDirData {
		return result, fmt.Errorf("存在包含 emptyDir 的 Pod，需要确认允许丢弃本地临时数据")
	}
	if !node.Spec.Unschedulable {
		node.Spec.Unschedulable = true
	}
	markNodeDrained(node)
	if _, err := c.Clientset.CoreV1().Nodes().Update(c.Ctx(), node, metav1.UpdateOptions{}); err != nil {
		return result, fmt.Errorf("标记节点不可调度失败: %w", err)
	}

	gracePeriodSeconds := int64(0)
	candidates := append(append([]DrainPod{}, plan.Evictable...), plan.RequiresEmptyDirConfirmation...)
	for _, item := range candidates {
		if err := c.Clientset.CoreV1().Pods(item.Namespace).Delete(c.Ctx(), item.Name, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriodSeconds}); err != nil {
			item.Reason = "强制删除请求失败: " + strings.TrimSpace(err.Error())
			result.Failed = append(result.Failed, item)
			continue
		}
		item.Reason = "已提交强制删除，控制器将在健康节点重建"
		result.Deleted = append(result.Deleted, item)
	}
	return result, nil
}

func (c *Client) NodeRemovalCheck(name string) (*NodeRemovalCheck, error) {
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取节点失败: %w", err)
	}
	check := &NodeRemovalCheck{NodeName: name}
	if !node.Spec.Unschedulable {
		check.Blockers = append(check.Blockers, DrainPod{Reason: "节点尚未 cordon，请先完成驱逐"})
	}
	if nodeReady(node) {
		check.Blockers = append(check.Blockers, DrainPod{Reason: "节点仍处于就绪状态，请先停止该节点的 K3s 服务"})
	}
	plan, err := c.DrainPlan(name)
	if err != nil {
		return nil, err
	}
	check.Blockers = append(check.Blockers, plan.Evictable...)
	check.Blockers = append(check.Blockers, plan.RequiresEmptyDirConfirmation...)
	check.Blockers = append(check.Blockers, plan.Blocked...)
	check.CanRemove = len(check.Blockers) == 0
	return check, nil
}

// DeleteNode only deletes a stopped, cordoned, fully drained Node object. Stopping
// k3s or k3s-agent remains an explicit host operation outside this API.
func (c *Client) DeleteNode(name string) error {
	check, err := c.NodeRemovalCheck(name)
	if err != nil {
		return err
	}
	if !check.CanRemove {
		return fmt.Errorf("节点尚未满足移出条件: %s", check.Blockers[0].Reason)
	}
	return c.Clientset.CoreV1().Nodes().Delete(c.Ctx(), name, metav1.DeleteOptions{})
}

// RejoinNode makes a previously drained node schedulable again. It only
// changes the Node object; K3s installation and Node registration are kept
// outside this reversible operation.
func (c *Client) RejoinNode(name string) (*NodeInfo, error) {
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取节点失败: %w", err)
	}
	if !node.Spec.Unschedulable && node.Annotations[nodeDrainAnnotation] == "" {
		info := nodeToInfo(node)
		return &info, nil
	}
	node.Spec.Unschedulable = false
	if node.Annotations != nil {
		delete(node.Annotations, nodeDrainAnnotation)
	}
	updated, err := c.Clientset.CoreV1().Nodes().Update(c.Ctx(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("恢复节点调度失败: %w", err)
	}
	info := nodeToInfo(updated)
	return &info, nil
}

func markNodeDrained(node *corev1.Node) {
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[nodeDrainAnnotation] = time.Now().UTC().Format(time.RFC3339)
}

func drainPod(pod *corev1.Pod) DrainPod {
	item := DrainPod{Namespace: pod.Namespace, Name: pod.Name}
	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && *owner.Controller {
			item.OwnerKind = owner.Kind
			return item
		}
	}
	return item
}

func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" && (owner.Controller == nil || *owner.Controller) {
			return true
		}
	}
	return false
}

func isControlPlaneNode(node *corev1.Node) bool {
	_, controlPlane := node.Labels["node-role.kubernetes.io/control-plane"]
	_, master := node.Labels["node-role.kubernetes.io/master"]
	return controlPlane || master
}

func isPDBViolation(err error) bool {
	if !apierrors.IsTooManyRequests(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "disruption budget") || strings.Contains(message, "poddisruptionbudget")
}

func podUsesEmptyDir(pod *corev1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.EmptyDir != nil {
			return true
		}
	}
	return false
}

func nodeToInfo(node *corev1.Node) NodeInfo {
	info := NodeInfo{Name: node.Name, Version: node.Status.NodeInfo.KubeletVersion, CreatedAt: node.CreationTimestamp.Format(time.RFC3339), OS: node.Status.NodeInfo.OSImage}
	health := nodeHealth(node, time.Now())
	info.Ready = health.State == NodeHealthReady
	info.Evicted = node.Spec.Unschedulable
	info.HealthState = health.State
	info.HealthReason = health.Reason
	info.LastHeartbeatAt = health.LastHeartbeatAt
	if isControlPlaneNode(node) {
		info.Roles = "control-plane"
	} else {
		info.Roles = "worker"
	}
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			info.InternalIP = addr.Address
			break
		}
	}
	info.CPUCores = node.Status.Capacity.Cpu().Value()
	info.MemoryMB = node.Status.Capacity.Memory().Value() / (1024 * 1024)
	return info
}

type nodeHealthInfo struct {
	State           string
	Reason          string
	LastHeartbeatAt string
}

func nodeHealth(node *corev1.Node, now time.Time) nodeHealthInfo {
	for index := range node.Status.Conditions {
		condition := &node.Status.Conditions[index]
		if condition.Type != corev1.NodeReady {
			continue
		}
		result := nodeHealthInfo{Reason: condition.Reason}
		heartbeat := condition.LastHeartbeatTime
		if heartbeat.IsZero() {
			heartbeat = condition.LastTransitionTime
		}
		if !heartbeat.IsZero() {
			result.LastHeartbeatAt = heartbeat.UTC().Format(time.RFC3339)
		}
		switch condition.Status {
		case corev1.ConditionTrue:
			result.State = NodeHealthReady
		case corev1.ConditionUnknown:
			if !heartbeat.IsZero() && now.Sub(heartbeat.Time) > nodeFailureThreshold {
				result.State = NodeHealthFailed
			} else {
				result.State = NodeHealthNotReady
			}
		default:
			result.State = NodeHealthNotReady
		}
		return result
	}
	return nodeHealthInfo{State: NodeHealthNotReady, Reason: "未报告 Ready 状态"}
}
