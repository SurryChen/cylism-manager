package k8s

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeInfo 节点展示信息
type NodeInfo struct {
	Name      string `json:"name"`
	Status    string `json:"status"`   // Ready / NotReady
	Role      string `json:"role"`     // control-plane / worker
	Version   string `json:"version"`
	CPUCores  int64  `json:"cpu_cores"`
	MemoryMB  int64  `json:"memory_mb"`
	CreatedAt string `json:"created_at"`
}

// ListNodeInfos 列出所有节点
func (c *Client) ListNodeInfos() ([]NodeInfo, error) {
	nodes, err := c.Clientset.CoreV1().Nodes().List(c.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var result []NodeInfo
	for _, node := range nodes.Items {
		info := nodeToInfo(&node)
		result = append(result, info)
	}
	return result, nil
}

// GetNodeInfo 获取单个节点信息
func (c *Client) GetNodeInfo(name string) (*NodeInfo, error) {
	node, err := c.Clientset.CoreV1().Nodes().Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get node %s: %w", name, err)
	}
	info := nodeToInfo(node)
	return &info, nil
}

// DrainNode 驱逐节点
func (c *Client) DrainNode(name string) error {
	// 标记节点不可调度
	node, err := c.Clientset.CoreV1().Nodes().Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	node.Spec.Unschedulable = true
	_, err = c.Clientset.CoreV1().Nodes().Update(c.ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("cordon node: %w", err)
	}

	// 驱逐 Pod（简化版本：删除节点上所有非 DaemonSet Pod）
	pods, err := c.Clientset.CoreV1().Pods("").List(c.ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + name,
	})
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	for _, pod := range pods.Items {
		// 跳过 DaemonSet Pod
		isDaemonSet := false
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "DaemonSet" {
				isDaemonSet = true
				break
			}
		}
		if isDaemonSet {
			continue
		}
		c.Clientset.CoreV1().Pods(pod.Namespace).Delete(c.ctx, pod.Name, metav1.DeleteOptions{})
	}

	return nil
}

// DeleteNode 从集群删除节点
func (c *Client) DeleteNode(name string) error {
	return c.Clientset.CoreV1().Nodes().Delete(c.ctx, name, metav1.DeleteOptions{})
}

func nodeToInfo(node *corev1.Node) NodeInfo {
	info := NodeInfo{
		Name:      node.Name,
		Version:   node.Status.NodeInfo.KubeletVersion,
		CreatedAt: node.CreationTimestamp.Format(time.RFC3339),
	}

	// 状态
	info.Status = "NotReady"
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			info.Status = "Ready"
		}
	}

	// 角色
	if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
		info.Role = "control-plane"
	} else if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
		info.Role = "control-plane"
	} else {
		info.Role = "worker"
	}

	// 资源
	info.CPUCores = node.Status.Capacity.Cpu().Value()
	info.MemoryMB = node.Status.Capacity.Memory().Value() / (1024 * 1024)

	return info
}
