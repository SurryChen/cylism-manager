package database

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type ClusterPod struct {
	Name         string                      `json:"name"`
	Component    string                      `json:"component"`
	DatabaseRole string                      `json:"database_role,omitempty"`
	Address      string                      `json:"address,omitempty"`
	NodeName     string                      `json:"node_name"`
	Phase        string                      `json:"phase"`
	Ready        bool                        `json:"ready"`
	Reason       string                      `json:"reason,omitempty"`
	Message      string                      `json:"message,omitempty"`
	LastEvent    string                      `json:"last_event,omitempty"`
	Images       []string                    `json:"images"`
	Claims       []string                    `json:"claims"`
	Retryable    bool                        `json:"retryable"`
	CPUMilli     *int64                      `json:"cpu_millicores,omitempty"`
	MemoryBytes  *int64                      `json:"memory_bytes,omitempty"`
	Containers   []k8s.RuntimeContainerUsage `json:"container_usage,omitempty"`
}

type ClusterClaim struct {
	Name         string `json:"name"`
	Phase        string `json:"phase"`
	StorageClass string `json:"storage_class,omitempty"`
	Capacity     string `json:"capacity,omitempty"`
	VolumeName   string `json:"volume_name,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type ClusterSnapshot struct {
	Status             string         `json:"status"`
	OnlineInstances    int            `json:"online_instances"`
	RouterReady        int            `json:"router_ready"`
	RouterReadWrite    string         `json:"router_read_write,omitempty"`
	RouterReadOnly     string         `json:"router_read_only,omitempty"`
	ConnectionChecked  bool           `json:"connection_details_checked"`
	Pods               []ClusterPod   `json:"pods"`
	Claims             []ClusterClaim `json:"claims"`
	StatefulSetCreated bool           `json:"stateful_set_created"`
	Progress           string         `json:"progress"`
	Warning            string         `json:"warning,omitempty"`
	WarningAt          time.Time      `json:"-"`
	Failure            string         `json:"failure,omitempty"`
	FailureAt          time.Time      `json:"-"`
}

func Snapshot(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) (*ClusterSnapshot, error) {
	status, online, routers, err := ClusterStatus(ctx, client, item)
	if err != nil {
		return nil, err
	}
	snapshot := &ClusterSnapshot{Status: status, OnlineInstances: online, RouterReady: routers, Pods: []ClusterPod{}, Claims: []ClusterClaim{}}
	if status == "missing" {
		snapshot.Progress = "等待 MySQL Operator 创建集群资源"
		return snapshot, nil
	}
	resource, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		conditions, _, _ := unstructured.NestedSlice(resource.Object, "status", "conditions")
		for _, value := range conditions {
			condition, ok := value.(map[string]any)
			if !ok || condition["status"] != "True" {
				continue
			}
			if kind, _ := condition["type"].(string); kind == "Error" || kind == "Failed" {
				snapshot.Failure, _ = condition["message"].(string)
				if snapshot.Failure == "" {
					snapshot.Failure, _ = condition["reason"].(string)
				}
			}
		}
	}
	stateful, err := client.Clientset.AppsV1().StatefulSets(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	snapshot.StatefulSetCreated = err == nil && (stateful.Labels["mysql.oracle.com/cluster"] == item.ResourceName || stateful.Name == item.ResourceName)
	services, err := client.Clientset.CoreV1().Services(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	snapshot.ConnectionChecked = true
	instanceService := ""
	for _, service := range services.Items {
		switch service.Name {
		case item.ResourceName + "-instances":
			if service.Spec.ClusterIP == corev1.ClusterIPNone {
				instanceService = service.Name
			}
		case item.ResourceName:
			for _, port := range service.Spec.Ports {
				address := net.JoinHostPort(fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, item.Namespace), fmt.Sprint(port.Port))
				switch port.Port {
				case 6446:
					snapshot.RouterReadWrite = address
				case 6447:
					snapshot.RouterReadOnly = address
				}
			}
		}
	}
	podList, err := client.Clientset.CoreV1().Pods(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	usage, _ := client.ListRuntimePodMetrics(ctx, item.Namespace)
	for _, pod := range podList.Items {
		if pod.Labels["mysql.oracle.com/cluster"] != item.ResourceName && !strings.HasPrefix(pod.Name, item.ResourceName+"-") {
			continue
		}
		component := pod.Labels["component"]
		if component != "mysqld" && component != "mysqlrouter" {
			if strings.Contains(pod.Name, "router") {
				component = "mysqlrouter"
			} else {
				component = "mysqld"
			}
		}
		entry := ClusterPod{Name: pod.Name, Component: component, DatabaseRole: pod.Labels["mysql.oracle.com/cluster-role"], NodeName: pod.Spec.NodeName, Phase: string(pod.Status.Phase), Images: []string{}, Claims: []string{}}
		if metric, ok := usage[pod.Name]; ok {
			entry.CPUMilli, entry.MemoryBytes = &metric.CPUMilli, &metric.MemoryBytes
			entry.Containers = metric.Containers
		}
		if component == "mysqld" && pod.Status.PodIP != "" {
			entry.Address = net.JoinHostPort(pod.Status.PodIP, "3306")
			if instanceService != "" {
				entry.Address = net.JoinHostPort(fmt.Sprintf("%s.%s.%s.svc.cluster.local", pod.Name, instanceService, item.Namespace), "3306")
			}
		}
		for _, container := range pod.Spec.InitContainers {
			entry.Images = append(entry.Images, container.Image)
		}
		for _, container := range pod.Spec.Containers {
			entry.Images = append(entry.Images, container.Image)
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil {
				entry.Claims = append(entry.Claims, volume.PersistentVolumeClaim.ClaimName)
			}
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				entry.Ready = true
			}
		}
		for _, container := range pod.Status.InitContainerStatuses {
			if container.State.Waiting != nil {
				entry.Reason = container.State.Waiting.Reason
				entry.Message = container.State.Waiting.Message
				break
			}
			if container.State.Terminated != nil && container.State.Terminated.ExitCode != 0 {
				entry.Reason = container.State.Terminated.Reason
				entry.Message = container.State.Terminated.Message
				break
			}
		}
		if entry.Reason == "" {
			for _, container := range pod.Status.ContainerStatuses {
				if container.State.Waiting != nil {
					entry.Reason = container.State.Waiting.Reason
					entry.Message = container.State.Waiting.Message
					break
				}
				if container.State.Terminated != nil && container.State.Terminated.ExitCode != 0 {
					entry.Reason = container.State.Terminated.Reason
					entry.Message = container.State.Terminated.Message
					break
				}
			}
		}
		if entry.Reason == "" {
			entry.Reason = pod.Status.Reason
			entry.Message = pod.Status.Message
		}
		entry.Retryable = !entry.Ready && pod.Spec.NodeName != "" && len(pod.OwnerReferences) > 0 && retryablePodReason(entry.Reason)
		snapshot.Pods = append(snapshot.Pods, entry)
	}
	sort.Slice(snapshot.Pods, func(i, j int) bool { return snapshot.Pods[i].Name < snapshot.Pods[j].Name })
	claims, err := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	usedClaims := map[string]bool{}
	for _, pod := range snapshot.Pods {
		for _, name := range pod.Claims {
			usedClaims[name] = true
		}
	}
	for _, claim := range claims.Items {
		if !usedClaims[claim.Name] && claim.Labels["mysql.oracle.com/cluster"] != item.ResourceName && !strings.Contains(claim.Name, item.ResourceName+"-") {
			continue
		}
		entry := ClusterClaim{Name: claim.Name, Phase: string(claim.Status.Phase), VolumeName: claim.Spec.VolumeName}
		if claim.Spec.StorageClassName != nil {
			entry.StorageClass = *claim.Spec.StorageClassName
		}
		if size, ok := claim.Status.Capacity[corev1.ResourceStorage]; ok {
			entry.Capacity = size.String()
		}
		snapshot.Claims = append(snapshot.Claims, entry)
	}
	sort.Slice(snapshot.Claims, func(i, j int) bool { return snapshot.Claims[i].Name < snapshot.Claims[j].Name })
	snapshot.Progress = fmt.Sprintf("数据库成员已就绪 %d/%d，Router 已就绪 %d/%d", online, DatabaseInstanceCount(item), routers, RouterInstanceCount(item))
	if status == "ready" && len(snapshot.Pods) == 0 {
		snapshot.Progress = "集群工作负载已就绪，尚未读取到对应的 Pod"
		return snapshot, nil
	}
	if status == "ready" {
		snapshot.Progress = "MySQL 集群已就绪"
		return snapshot, nil
	}
	events, err := client.Clientset.CoreV1().Events(item.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return snapshot, nil
	}
	var latest time.Time
	resourceWarnings := map[string]time.Time{}
	resourceEvents := map[string]time.Time{}
	knownResources := map[string]bool{item.ResourceName: true}
	for _, pod := range snapshot.Pods {
		knownResources[pod.Name] = true
	}
	for _, claim := range snapshot.Claims {
		knownResources[claim.Name] = true
	}
	for _, event := range events.Items {
		name := event.InvolvedObject.Name
		owned := knownResources[name] || strings.HasPrefix(name, item.ResourceName+"-")
		if !owned {
			continue
		}
		when := event.LastTimestamp.Time
		if when.IsZero() {
			when = event.EventTime.Time
		}
		if when.IsZero() {
			when = event.CreationTimestamp.Time
		}
		if !when.Before(resourceEvents[name]) {
			resourceEvents[name] = when
			for i := range snapshot.Pods {
				if snapshot.Pods[i].Name == name {
					snapshot.Pods[i].LastEvent = fmt.Sprintf("%s: %s", event.Reason, event.Message)
				}
			}
		}
		if event.Type != corev1.EventTypeWarning {
			continue
		}
		if when.After(latest) {
			latest = when
			snapshot.WarningAt = when
			snapshot.Warning = fmt.Sprintf("%s %s: %s", event.InvolvedObject.Kind, event.Reason, event.Message)
		}
		if !when.Before(resourceWarnings[name]) {
			resourceWarnings[name] = when
			for i := range snapshot.Pods {
				if snapshot.Pods[i].Name == name && !snapshot.Pods[i].Ready {
					snapshot.Pods[i].Message = event.Message
				}
			}
			for i := range snapshot.Claims {
				if snapshot.Claims[i].Name == name && snapshot.Claims[i].Phase != string(corev1.ClaimBound) {
					snapshot.Claims[i].Reason = event.Message
				}
			}
		}
		if event.InvolvedObject.Kind == "InnoDBCluster" && event.InvolvedObject.Name == item.ResourceName && event.Reason == "CreateResourceFailed" {
			if !when.Before(snapshot.FailureAt) {
				snapshot.Failure = "Operator 创建工作负载失败：" + event.Message
				snapshot.FailureAt = when
			}
		}
	}
	return snapshot, nil
}

func retryablePodReason(reason string) bool {
	switch reason {
	case "ImagePullBackOff", "ErrImagePull", "CreateContainerError", "CreateContainerConfigError", "CrashLoopBackOff", "Error", "Failed", "RunContainerError":
		return true
	}
	return false
}
