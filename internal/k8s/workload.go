package k8s

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1 "k8s.io/api/core/v1"
)

// --- Info types ---

// DeploymentInfo Deployment 展示信息
type DeploymentInfo struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Replicas  int32    `json:"replicas"`
	Ready     int32    `json:"ready"`
	Images    []string `json:"images"`
	CPU       string   `json:"cpu"`
	Memory    string   `json:"memory"`
	Age       string   `json:"age"`
}

// StatefulSetInfo StatefulSet 展示信息
type StatefulSetInfo struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Replicas  int32    `json:"replicas"`
	Ready     int32    `json:"ready"`
	Images    []string `json:"images"`
	Age       string   `json:"age"`
}

// DaemonSetInfo DaemonSet 展示信息
type DaemonSetInfo struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Desired      int32             `json:"desired"`
	Ready        int32             `json:"ready"`
	Images       []string          `json:"images"`
	NodeSelector map[string]string `json:"node_selector"`
	Age          string            `json:"age"`
}

// PodRef Pod 引用信息
type PodRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Node      string `json:"node"`
	IP        string `json:"ip"`
	Restarts  int32  `json:"restarts"`
	Age       string `json:"age"`
}

// RevisionInfo Deployment 版本历史
type RevisionInfo struct {
	Revision int64  `json:"revision"`
	Image    string `json:"image"`
	Replicas int32  `json:"replicas"`
	Age      string `json:"age"`
	Cause    string `json:"cause"`
}

// --- Deployment ---

// ListDeployments 列出所有 Deployment
func (c *Client) ListDeployments(ns string) ([]DeploymentInfo, error) {
	var list *appsv1.DeploymentList
	var err error
	if ns == "" {
		list, err = c.Clientset.AppsV1().Deployments("").List(c.ctx, metav1.ListOptions{})
	} else {
		list, err = c.Clientset.AppsV1().Deployments(ns).List(c.ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	result := make([]DeploymentInfo, 0, len(list.Items))
	for _, d := range list.Items {
		result = append(result, deploymentToInfo(&d))
	}
	return result, nil
}

// GetDeployment 获取单个 Deployment 详情
func (c *Client) GetDeployment(namespace, name string) (*DeploymentInfo, error) {
	d, err := c.Clientset.AppsV1().Deployments(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment %s/%s: %w", namespace, name, err)
	}
	info := deploymentToInfo(d)
	return &info, nil
}

// ListDeploymentPods 获取 Deployment 关联的 Pod
func (c *Client) ListDeploymentPods(namespace, name string) ([]PodRef, error) {
	d, err := c.Clientset.AppsV1().Deployments(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}

	selector := metav1.FormatLabelSelector(d.Spec.Selector)
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(c.ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	result := make([]PodRef, 0, len(pods.Items))
	for _, p := range pods.Items {
		ref := podRefFromPod(&p)
		result = append(result, ref)
	}
	return result, nil
}

// ListDeploymentRevisions 获取 Deployment 版本历史（通过 ReplicaSet）
func (c *Client) ListDeploymentRevisions(namespace, name string) ([]RevisionInfo, error) {
	d, err := c.Clientset.AppsV1().Deployments(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}

	selector := metav1.FormatLabelSelector(d.Spec.Selector)
	rsList, err := c.Clientset.AppsV1().ReplicaSets(namespace).List(c.ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("list replicasets: %w", err)
	}

	result := make([]RevisionInfo, 0, len(rsList.Items))
	for _, rs := range rsList.Items {
		if rs.Annotations == nil {
			continue
		}
		rev := rs.Annotations["deployment.kubernetes.io/revision"]
		if rev == "" {
			continue
		}
		image := ""
		if len(rs.Spec.Template.Spec.Containers) > 0 {
			image = rs.Spec.Template.Spec.Containers[0].Image
		}
		var revisionNum int64
		fmt.Sscanf(rev, "%d", &revisionNum)

		cause := ""
		if rs.Annotations["kubernetes.io/change-cause"] != "" {
			cause = rs.Annotations["kubernetes.io/change-cause"]
		}

		result = append(result, RevisionInfo{
			Revision: revisionNum,
			Image:    image,
			Replicas: safeDerefInt32(rs.Spec.Replicas),
			Age:      timeAgo(rs.CreationTimestamp.Time),
			Cause:    cause,
		})
	}
	return result, nil
}

// ScaleDeployment 扩缩容 Deployment
func (c *Client) ScaleDeployment(namespace, name string, replicas int32) error {
	scale, err := c.Clientset.AppsV1().Deployments(namespace).GetScale(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get scale: %w", err)
	}
	scale.Spec.Replicas = replicas

	_, err = c.Clientset.AppsV1().Deployments(namespace).UpdateScale(c.ctx, name, scale, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update scale: %w", err)
	}
	return nil
}

// UpdateDeploymentImage 更新 Deployment 容器镜像
func (c *Client) UpdateDeploymentImage(namespace, name, container, image string) error {
	patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"%s","image":"%s"}]}}}}`, container, image)
	_, err := c.Clientset.AppsV1().Deployments(namespace).Patch(
		c.ctx, name, types.StrategicMergePatchType,
		[]byte(patch), metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patch deployment: %w", err)
	}
	return nil
}

// RollbackDeployment 回滚 Deployment 到指定版本
func (c *Client) RollbackDeployment(namespace, name string, revision int64) error {
	d, err := c.Clientset.AppsV1().Deployments(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}

	if d.Annotations == nil {
		d.Annotations = make(map[string]string)
	}
	d.Annotations["kubectl.kubernetes.io/restartAt"] = time.Now().Format(time.RFC3339)

	// K8s 原生回滚通过 replica set annotation 实现
	// 我们通过设置 deployment.kubernetes.io/revision 触发回滚
	patch := fmt.Sprintf(`{"spec":{"rollbackTo":{"revision":%d}}}`, revision)
	_, err = c.Clientset.AppsV1().Deployments(namespace).Patch(
		c.ctx, name, types.StrategicMergePatchType,
		[]byte(patch), metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("rollback deployment: %w", err)
	}
	return nil
}

// --- StatefulSet ---

// ListStatefulSets 列出所有 StatefulSet
func (c *Client) ListStatefulSets(ns string) ([]StatefulSetInfo, error) {
	var list *appsv1.StatefulSetList
	var err error
	if ns == "" {
		list, err = c.Clientset.AppsV1().StatefulSets("").List(c.ctx, metav1.ListOptions{})
	} else {
		list, err = c.Clientset.AppsV1().StatefulSets(ns).List(c.ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}

	result := make([]StatefulSetInfo, 0, len(list.Items))
	for _, s := range list.Items {
		result = append(result, statefulSetToInfo(&s))
	}
	return result, nil
}

// GetStatefulSet 获取单个 StatefulSet 详情
func (c *Client) GetStatefulSet(namespace, name string) (*StatefulSetInfo, error) {
	s, err := c.Clientset.AppsV1().StatefulSets(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get statefulset %s/%s: %w", namespace, name, err)
	}
	info := statefulSetToInfo(s)
	return &info, nil
}

// ScaleStatefulSet 扩缩容 StatefulSet
func (c *Client) ScaleStatefulSet(namespace, name string, replicas int32) error {
	scale, err := c.Clientset.AppsV1().StatefulSets(namespace).GetScale(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get scale: %w", err)
	}
	scale.Spec.Replicas = replicas

	_, err = c.Clientset.AppsV1().StatefulSets(namespace).UpdateScale(c.ctx, name, scale, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update scale: %w", err)
	}
	return nil
}

// --- DaemonSet ---

// ListDaemonSets 列出所有 DaemonSet
func (c *Client) ListDaemonSets(ns string) ([]DaemonSetInfo, error) {
	var list *appsv1.DaemonSetList
	var err error
	if ns == "" {
		list, err = c.Clientset.AppsV1().DaemonSets("").List(c.ctx, metav1.ListOptions{})
	} else {
		list, err = c.Clientset.AppsV1().DaemonSets(ns).List(c.ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list daemonsets: %w", err)
	}

	result := make([]DaemonSetInfo, 0, len(list.Items))
	for _, d := range list.Items {
		result = append(result, daemonSetToInfo(&d))
	}
	return result, nil
}

// GetDaemonSet 获取单个 DaemonSet 详情
func (c *Client) GetDaemonSet(namespace, name string) (*DaemonSetInfo, error) {
	d, err := c.Clientset.AppsV1().DaemonSets(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get daemonset %s/%s: %w", namespace, name, err)
	}
	info := daemonSetToInfo(d)
	return &info, nil
}

// --- Helpers ---

func deploymentToInfo(d *appsv1.Deployment) DeploymentInfo {
	images := make([]string, 0, len(d.Spec.Template.Spec.Containers))
	var cpu, memory string
	for _, c := range d.Spec.Template.Spec.Containers {
		images = append(images, c.Image)
		if cpu == "" && c.Resources.Requests.Cpu() != nil {
			cpu = c.Resources.Requests.Cpu().String()
		}
		if memory == "" && c.Resources.Requests.Memory() != nil {
			memory = c.Resources.Requests.Memory().String()
		}
	}
	return DeploymentInfo{
		Name:      d.Name,
		Namespace: d.Namespace,
		Replicas:  safeDerefInt32(d.Spec.Replicas),
		Ready:     d.Status.ReadyReplicas,
		Images:    images,
		CPU:       cpu,
		Memory:    memory,
		Age:       timeAgo(d.CreationTimestamp.Time),
	}
}

func statefulSetToInfo(s *appsv1.StatefulSet) StatefulSetInfo {
	images := make([]string, 0, len(s.Spec.Template.Spec.Containers))
	for _, c := range s.Spec.Template.Spec.Containers {
		images = append(images, c.Image)
	}
	return StatefulSetInfo{
		Name:      s.Name,
		Namespace: s.Namespace,
		Replicas:  safeDerefInt32(s.Spec.Replicas),
		Ready:     s.Status.ReadyReplicas,
		Images:    images,
		Age:       timeAgo(s.CreationTimestamp.Time),
	}
}

func daemonSetToInfo(d *appsv1.DaemonSet) DaemonSetInfo {
	images := make([]string, 0, len(d.Spec.Template.Spec.Containers))
	for _, c := range d.Spec.Template.Spec.Containers {
		images = append(images, c.Image)
	}
	return DaemonSetInfo{
		Name:         d.Name,
		Namespace:    d.Namespace,
		Desired:      d.Status.DesiredNumberScheduled,
		Ready:        d.Status.NumberReady,
		Images:       images,
		NodeSelector: d.Spec.Template.Spec.NodeSelector,
		Age:          timeAgo(d.CreationTimestamp.Time),
	}
}

func podRefFromPod(p *corev1.Pod) PodRef {
	restarts := int32(0)
	for _, cs := range p.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}
	return PodRef{
		Name:      p.Name,
		Namespace: p.Namespace,
		Status:    string(p.Status.Phase),
		Node:      p.Spec.NodeName,
		IP:        p.Status.PodIP,
		Restarts:  restarts,
		Age:       timeAgo(p.CreationTimestamp.Time),
	}
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func safeDerefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
