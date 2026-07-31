package k8s

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const mirrorPodAnnotation = "kubernetes.io/config.mirror"

// NodeInfo is the node summary shown in the cluster console.
type NodeInfo struct {
	Name       string `json:"name"`
	Ready      bool   `json:"ready"`
	Roles      string `json:"roles"`
	Version    string `json:"version"`
	InternalIP string `json:"internal_ip"`
	OS         string `json:"os"`
	CPUCores   int64  `json:"cpu_cores"`
	MemoryMB   int64  `json:"memory_mb"`
	CreatedAt  string `json:"created_at"`
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

// DrainResult reports each eviction request. Pending entries were protected by a PDB
// or failed to be submitted and are deliberately left running on the source node.
type DrainResult struct {
	Plan    *DrainPlan `json:"plan"`
	Evicted []DrainPod `json:"evicted"`
	Pending []DrainPod `json:"pending"`
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
		if _, err := c.Clientset.CoreV1().Nodes().Update(c.Ctx(), node, metav1.UpdateOptions{}); err != nil {
			return nil, fmt.Errorf("标记节点不可调度失败: %w", err)
		}
	}

	candidates := append(append([]DrainPod{}, plan.Evictable...), plan.RequiresEmptyDirConfirmation...)
	result := &DrainResult{Plan: plan}
	for _, item := range candidates {
		err := c.Clientset.CoreV1().Pods(item.Namespace).EvictV1(c.Ctx(), &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: item.Name, Namespace: item.Namespace}})
		if err == nil {
			result.Evicted = append(result.Evicted, item)
			continue
		}
		if apierrors.IsTooManyRequests(err) {
			item.Reason = "PodDisruptionBudget 暂不允许驱逐: " + strings.TrimSpace(err.Error())
		} else {
			item.Reason = "提交驱逐请求失败: " + strings.TrimSpace(err.Error())
		}
		result.Pending = append(result.Pending, item)
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
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			info.Ready = true
		}
	}
	if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
		info.Roles = "control-plane"
	} else if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
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
