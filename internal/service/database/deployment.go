package database

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentNode struct {
	Name                string `json:"name"`
	InternalIP          string `json:"internal_ip"`
	Ready               bool   `json:"ready"`
	Evicted             bool   `json:"evicted"`
	DefaultNoSchedule   bool   `json:"default_no_schedule"`
	DiskPressure        bool   `json:"disk_pressure"`
	DatabaseAllowed     bool   `json:"database_allowed"`
	RouterAllowed       bool   `json:"router_allowed"`
	BlockedReason       string `json:"blocked_reason,omitempty"`
	RouterBlockedReason string `json:"router_blocked_reason,omitempty"`
}

func tolerableDatabaseTaint(taint corev1.Taint) bool {
	return taint.Key == databaseNoScheduleTaintKey && taint.Value == "true" && taint.Effect == corev1.TaintEffectNoSchedule
}

func DeploymentNodes(ctx context.Context, client *k8s.Client) ([]DeploymentNode, error) {
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	result := make([]DeploymentNode, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		entry := DeploymentNode{Name: node.Name, Evicted: node.Spec.Unschedulable}
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP {
				entry.InternalIP = address.Address
				break
			}
		}
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				entry.Ready = true
			}
			if condition.Type == corev1.NodeDiskPressure && condition.Status == corev1.ConditionTrue {
				entry.DiskPressure = true
			}
		}
		if !entry.Ready {
			entry.BlockedReason = "节点未就绪"
		}
		if entry.Evicted {
			entry.BlockedReason = "节点已暂停调度"
		}
		if entry.DiskPressure {
			entry.BlockedReason = "节点磁盘压力过高"
		}
		for _, taint := range node.Spec.Taints {
			if tolerableDatabaseTaint(taint) {
				entry.DefaultNoSchedule = true
				continue
			}
			if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
				entry.BlockedReason = "禁止调度污点：" + taint.Key
			}
		}
		entry.DatabaseAllowed = entry.BlockedReason == ""
		entry.RouterBlockedReason = entry.BlockedReason
		if entry.RouterBlockedReason == "" && entry.DefaultNoSchedule {
			entry.RouterBlockedReason = "禁止普通应用调度"
		}
		entry.RouterAllowed = entry.RouterBlockedReason == ""
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func ValidatePrimaryNode(name string, nodes []string) error {
	if name == "" {
		return nil
	}
	for _, node := range nodes {
		if name == node {
			return nil
		}
	}
	return fmt.Errorf("优先主节点必须是选中的数据库节点")
}

// ResolveStorageClass prevents a misspelled class or a missing default from
// leaving PVCs pending indefinitely after the deployment has been submitted.
func ResolveStorageClass(ctx context.Context, client *k8s.Client, name string) (string, error) {
	classes, err := client.Clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("读取数据存储类型：%w", err)
	}
	var defaults []string
	for _, class := range classes.Items {
		if name != "" && class.Name == name {
			return name, nil
		}
		if class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" || class.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			defaults = append(defaults, class.Name)
		}
	}
	if name != "" {
		return "", fmt.Errorf("数据存储类型 %s 不存在", name)
	}
	if len(defaults) == 1 {
		return defaults[0], nil
	}
	if len(defaults) == 0 {
		return "", fmt.Errorf("集群没有默认数据存储类型，请选择一个已有的存储类型")
	}
	return "", fmt.Errorf("集群存在多个默认存储类型（%s），请明确选择", strings.Join(defaults, "、"))
}
