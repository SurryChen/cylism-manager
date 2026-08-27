package k8s

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	ManagedByLabel                = "app.kubernetes.io/managed-by"
	ManagedByValue                = "cylism-manager"
	EnvironmentLabel              = "cylism.io/environment"
	InfrastructureLabel           = "cylism.io/infrastructure"
	InfrastructureAlertmanager    = "alertmanager"
	InfrastructureVictoriaMetrics = "victoria-metrics"
	InfrastructureLoki            = "loki"
	InfrastructureRuntime         = "agent-runtime"
	InfrastructureOCIRegistry     = "oci-registry"
	managedOCIRegistryNamespace   = "cylism-system"
	managedOCIRegistryPVCName     = "cylism-oci-registry-data"
)

// PersistentVolumeClaimRequest contains the user-controlled fields supported
// by the platform. The first release deliberately creates RWO claims only.
type PersistentVolumeClaimRequest struct {
	Name             string `json:"name"`
	Storage          string `json:"storage"`
	StorageClassName string `json:"storage_class_name,omitempty"`
}

// PersistentVolumeClaimInfo is a Kubernetes-derived summary for the storage UI
// and application release validation.
type PersistentVolumeClaimInfo struct {
	Name                 string   `json:"name"`
	Namespace            string   `json:"namespace"`
	Managed              bool     `json:"managed"`
	EnvironmentID        uint     `json:"environment_id,omitempty"`
	OwnerType            string   `json:"owner_type"`
	Owner                string   `json:"owner,omitempty"`
	OwnerName            string   `json:"owner_name,omitempty"`
	ReadOnly             bool     `json:"read_only"`
	Phase                string   `json:"phase"`
	Storage              string   `json:"storage"`
	StorageClassName     string   `json:"storage_class_name,omitempty"`
	VolumeName           string   `json:"volume_name,omitempty"`
	AccessModes          []string `json:"access_modes"`
	BoundNode            string   `json:"bound_node,omitempty"`
	LocalPath            string   `json:"-"`
	IsLocal              bool     `json:"is_local"`
	ReclaimPolicy        string   `json:"reclaim_policy,omitempty"`
	WaitForFirstConsumer bool     `json:"wait_for_first_consumer"`
	CreationTimestamp    string   `json:"created_at,omitempty"`
}

type StorageClassInfo struct {
	Name              string `json:"name"`
	Provisioner       string `json:"provisioner"`
	IsDefault         bool   `json:"is_default"`
	VolumeBindingMode string `json:"volume_binding_mode"`
	ReclaimPolicy     string `json:"reclaim_policy"`
}

func EnvironmentLabelValue(environmentID uint) string {
	return fmt.Sprintf("environment-%d", environmentID)
}

func (c *Client) ListManagedPVCs(namespace string, environmentID uint) ([]PersistentVolumeClaimInfo, error) {
	claims, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(c.Ctx(), metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", ManagedByLabel, ManagedByValue)})
	if err != nil {
		return nil, fmt.Errorf("list persistentvolumeclaims: %w", err)
	}
	managedClaims := make([]corev1.PersistentVolumeClaim, 0, len(claims.Items))
	for index := range claims.Items {
		if !isManagedPVC(&claims.Items[index], environmentID) {
			continue
		}
		managedClaims = append(managedClaims, claims.Items[index])
	}
	result, err := c.pvcInfos(managedClaims)
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// ListPVCs returns every claim in the requested namespace, or cluster-wide
// when namespace is empty. It intentionally includes externally created
// claims so the infrastructure inventory is not limited to applications.
func (c *Client) ListPVCs(namespace string) ([]PersistentVolumeClaimInfo, error) {
	claims, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list persistentvolumeclaims: %w", err)
	}
	result, err := c.pvcInfos(claims.Items)
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (c *Client) GetManagedPVC(namespace, name string, environmentID uint) (*PersistentVolumeClaimInfo, error) {
	claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get persistentvolumeclaim %s/%s: %w", namespace, name, err)
	}
	if !isManagedPVC(claim, environmentID) {
		return nil, fmt.Errorf("PVC %q 不属于当前环境或未由平台管理", name)
	}
	if infrastructurePVCOwner(claim) != "" {
		return nil, fmt.Errorf("PVC %q 由基础设施组件 %s 管理，不能通过通用存储接口修改", name, infrastructurePVCOwnerName(infrastructurePVCOwner(claim)))
	}
	info, err := c.pvcInfo(claim)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// GetPVCInfo resolves local-path metadata for a known PVC. Callers remain
// responsible for applying their own platform ownership boundary.
func (c *Client) GetPVCInfo(namespace, name string) (*PersistentVolumeClaimInfo, error) {
	claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get persistentvolumeclaim %s/%s: %w", namespace, name, err)
	}
	info, err := c.pvcInfo(claim)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) CreateManagedPVC(namespace string, environmentID uint, request PersistentVolumeClaimRequest) (*corev1.PersistentVolumeClaim, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" || len(validation.IsDNS1123Label(name)) > 0 {
		return nil, fmt.Errorf("PVC 名称必须是合法的 Kubernetes 名称")
	}
	storage, err := resource.ParseQuantity(strings.TrimSpace(request.Storage))
	if err != nil || storage.Sign() <= 0 {
		return nil, fmt.Errorf("存储容量格式无效")
	}
	labels := map[string]string{ManagedByLabel: ManagedByValue}
	if environmentID != 0 {
		labels[EnvironmentLabel] = EnvironmentLabelValue(environmentID)
	}
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: storage}},
		},
	}
	if storageClassName := strings.TrimSpace(request.StorageClassName); storageClassName != "" {
		claim.Spec.StorageClassName = &storageClassName
	}
	created, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Create(c.Ctx(), claim, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create persistentvolumeclaim: %w", err)
	}
	return created, nil
}

// CreatePVCBindingPod schedules a temporary holder so WaitForFirstConsumer can
// provision a local PVC on the requested node before data is copied.
func (c *Client) CreatePVCBindingPod(namespace, migrationID, claimName, nodeName, image string) (*corev1.Pod, error) {
	if strings.TrimSpace(migrationID) == "" || strings.TrimSpace(claimName) == "" || strings.TrimSpace(nodeName) == "" || strings.TrimSpace(image) == "" {
		return nil, fmt.Errorf("迁移绑定 Pod 参数不完整")
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "cylism-pvc-migrate-" + migrationID, Namespace: namespace, Labels: map[string]string{ManagedByLabel: ManagedByValue, "cylism.io/pvc-migration": migrationID}},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			NodeSelector:  map[string]string{corev1.LabelHostname: nodeName},
			Containers:    []corev1.Container{{Name: "holder", Image: image, VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}}}},
			Volumes:       []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName}}}},
		},
	}
	created, err := c.Clientset.CoreV1().Pods(namespace).Create(c.Ctx(), pod, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return c.Clientset.CoreV1().Pods(namespace).Get(c.Ctx(), pod.Name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("create PVC binding pod: %w", err)
	}
	return created, nil
}

// WaitForPVCBound waits for a specifically named claim without assuming it is
// application-owned. Infrastructure migrations use this alongside their own
// fixed-resource ownership checks.
func (c *Client) WaitForPVCBound(ctx context.Context, namespace, name string) (*PersistentVolumeClaimInfo, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		claim, err := c.GetPVCInfo(namespace, name)
		if err != nil {
			return nil, err
		}
		if claim.Phase == string(corev1.ClaimBound) {
			return claim, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("等待目标 PVC 绑定: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *Client) DeletePVCBindingPod(namespace, migrationID string) error {
	name := "cylism-pvc-migrate-" + migrationID
	err := c.Clientset.CoreV1().Pods(namespace).Delete(c.Ctx(), name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete PVC binding pod: %w", err)
	}
	return nil
}

func (c *Client) WaitForManagedPVCBound(ctx context.Context, namespace, name string, environmentID uint) (*PersistentVolumeClaimInfo, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		claim, err := c.GetManagedPVC(namespace, name, environmentID)
		if err != nil {
			return nil, err
		}
		if claim.Phase == string(corev1.ClaimBound) {
			return claim, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("等待目标 PVC 绑定: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *Client) ReplaceDeploymentPVCNode(namespace, name, sourceClaim, targetClaim, targetNode string) (int32, error) {
	deployment, err := c.Clientset.AppsV1().Deployments(namespace).Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("get deployment: %w", err)
	}
	if deployment.Labels[ManagedByLabel] != ManagedByValue {
		return 0, fmt.Errorf("Deployment %q 未由平台管理", name)
	}
	updated := false
	for index := range deployment.Spec.Template.Spec.Volumes {
		volume := &deployment.Spec.Template.Spec.Volumes[index]
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == sourceClaim {
			volume.PersistentVolumeClaim.ClaimName = targetClaim
			updated = true
		}
	}
	if !updated {
		return 0, fmt.Errorf("Deployment %q 未引用 PVC %q", name, sourceClaim)
	}
	if deployment.Spec.Template.Spec.NodeSelector == nil {
		deployment.Spec.Template.Spec.NodeSelector = map[string]string{}
	}
	deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] = targetNode
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	if _, err := c.Clientset.AppsV1().Deployments(namespace).Update(c.Ctx(), deployment, metav1.UpdateOptions{}); err != nil {
		return 0, fmt.Errorf("update deployment PVC: %w", err)
	}
	return replicas, nil
}

func (c *Client) DeploymentUsingPVC(namespace, claimName string) ([]appsv1.Deployment, error) {
	deployments, err := c.Clientset.AppsV1().Deployments(namespace).List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	result := make([]appsv1.Deployment, 0)
	for index := range deployments.Items {
		for _, volume := range deployments.Items[index].Spec.Template.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claimName {
				result = append(result, deployments.Items[index])
				break
			}
		}
	}
	return result, nil
}

func (c *Client) DeleteManagedPVC(namespace, name string, environmentID uint) error {
	claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get persistentvolumeclaim %s/%s: %w", namespace, name, err)
	}
	if !isManagedPVC(claim, environmentID) {
		return fmt.Errorf("PVC %q 不属于当前环境或未由平台管理", name)
	}
	if owner := infrastructurePVCOwner(claim); owner != "" {
		return fmt.Errorf("PVC %q 由基础设施组件 %s 管理，不能通过通用存储接口删除", name, infrastructurePVCOwnerName(owner))
	}
	if err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(c.Ctx(), name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("delete persistentvolumeclaim: %w", err)
	}
	return nil
}

func (c *Client) ListStorageClasses() ([]StorageClassInfo, error) {
	classes, err := c.Clientset.StorageV1().StorageClasses().List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list storageclasses: %w", err)
	}
	result := make([]StorageClassInfo, 0, len(classes.Items))
	for index := range classes.Items {
		class := &classes.Items[index]
		result = append(result, StorageClassInfo{
			Name:              class.Name,
			Provisioner:       class.Provisioner,
			IsDefault:         class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" || class.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true",
			VolumeBindingMode: stringValue(class.VolumeBindingMode),
			ReclaimPolicy:     reclaimPolicyValue(class.ReclaimPolicy),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDefault != result[j].IsDefault {
			return result[i].IsDefault
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (c *Client) pvcInfo(claim *corev1.PersistentVolumeClaim) (PersistentVolumeClaimInfo, error) {
	var storageClass *storagev1.StorageClass
	if name := valueOrEmpty(claim.Spec.StorageClassName); name != "" {
		current, err := c.Clientset.StorageV1().StorageClasses().Get(c.Ctx(), name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return PersistentVolumeClaimInfo{}, fmt.Errorf("get storageclass %s: %w", name, err)
		}
		storageClass = current
	}
	var volume *corev1.PersistentVolume
	if claim.Spec.VolumeName != "" {
		current, err := c.Clientset.CoreV1().PersistentVolumes().Get(c.Ctx(), claim.Spec.VolumeName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return PersistentVolumeClaimInfo{}, fmt.Errorf("get persistentvolume %s: %w", claim.Spec.VolumeName, err)
		}
		volume = current
	}
	return pvcInfoFromResources(claim, storageClass, volume), nil
}

func (c *Client) pvcInfos(claims []corev1.PersistentVolumeClaim) ([]PersistentVolumeClaimInfo, error) {
	if len(claims) == 0 {
		return []PersistentVolumeClaimInfo{}, nil
	}
	storageClasses, err := c.Clientset.StorageV1().StorageClasses().List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list storageclasses: %w", err)
	}
	classesByName := make(map[string]*storagev1.StorageClass, len(storageClasses.Items))
	for index := range storageClasses.Items {
		classesByName[storageClasses.Items[index].Name] = &storageClasses.Items[index]
	}
	volumes, err := c.Clientset.CoreV1().PersistentVolumes().List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list persistentvolumes: %w", err)
	}
	volumesByName := make(map[string]*corev1.PersistentVolume, len(volumes.Items))
	for index := range volumes.Items {
		volumesByName[volumes.Items[index].Name] = &volumes.Items[index]
	}
	result := make([]PersistentVolumeClaimInfo, 0, len(claims))
	for index := range claims {
		result = append(result, pvcInfoFromResources(&claims[index], classesByName[valueOrEmpty(claims[index].Spec.StorageClassName)], volumesByName[claims[index].Spec.VolumeName]))
	}
	return result, nil
}

func pvcInfoFromResources(claim *corev1.PersistentVolumeClaim, storageClass *storagev1.StorageClass, volume *corev1.PersistentVolume) PersistentVolumeClaimInfo {
	info := PersistentVolumeClaimInfo{
		Name:              claim.Name,
		Namespace:         claim.Namespace,
		Phase:             string(claim.Status.Phase),
		StorageClassName:  valueOrEmpty(claim.Spec.StorageClassName),
		VolumeName:        claim.Spec.VolumeName,
		CreationTimestamp: claim.CreationTimestamp.UTC().Format("2006-01-02T15:04:05Z"),
	}
	info.Managed, info.EnvironmentID = managedPVCEnvironmentID(claim)
	if owner := infrastructurePVCOwner(claim); owner != "" {
		info.OwnerType = "infrastructure"
		info.Owner = owner
		info.OwnerName = infrastructurePVCOwnerName(owner)
		info.ReadOnly = true
	} else if info.Managed {
		info.OwnerType = "application"
	} else {
		info.OwnerType = "external"
	}
	if storage := claim.Status.Capacity.Storage(); storage != nil && !storage.IsZero() {
		info.Storage = storage.String()
	} else if storage := claim.Spec.Resources.Requests.Storage(); storage != nil {
		info.Storage = storage.String()
	}
	for _, mode := range claim.Spec.AccessModes {
		info.AccessModes = append(info.AccessModes, string(mode))
	}
	if storageClass != nil && storageClass.VolumeBindingMode != nil {
		info.WaitForFirstConsumer = *storageClass.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
	}
	if volume != nil {
		info.ReclaimPolicy = string(volume.Spec.PersistentVolumeReclaimPolicy)
		info.BoundNode = persistentVolumeNodeName(volume)
		info.LocalPath, info.IsLocal = persistentVolumeLocalPath(volume)
	}
	return info
}

func isManagedPVC(claim *corev1.PersistentVolumeClaim, environmentID uint) bool {
	managed, claimEnvironmentID := managedPVCEnvironmentID(claim)
	return managed && (claimEnvironmentID == environmentID || claimEnvironmentID == 0)
}

func managedPVCEnvironmentID(claim *corev1.PersistentVolumeClaim) (bool, uint) {
	if claim.Labels[ManagedByLabel] != ManagedByValue {
		return false, 0
	}
	value := strings.TrimPrefix(claim.Labels[EnvironmentLabel], "environment-")
	if value == "" || value == claim.Labels[EnvironmentLabel] {
		return true, 0
	}
	environmentID, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return true, 0
	}
	return true, uint(environmentID)
}

func infrastructurePVCOwner(claim *corev1.PersistentVolumeClaim) string {
	if claim == nil || claim.Labels[ManagedByLabel] != ManagedByValue {
		return ""
	}
	if claim.Namespace == managedOCIRegistryNamespace && claim.Name == managedOCIRegistryPVCName {
		return InfrastructureOCIRegistry
	}
	switch claim.Labels[InfrastructureLabel] {
	case InfrastructureAlertmanager, InfrastructureVictoriaMetrics, InfrastructureLoki, InfrastructureRuntime:
		return claim.Labels[InfrastructureLabel]
	default:
		return ""
	}
}

func infrastructurePVCOwnerName(owner string) string {
	switch owner {
	case InfrastructureAlertmanager:
		return "Alertmanager"
	case InfrastructureVictoriaMetrics:
		return "VictoriaMetrics"
	case InfrastructureLoki:
		return "Loki 日志存储"
	case InfrastructureRuntime:
		return "Agent Runtime"
	case InfrastructureOCIRegistry:
		return "OCI 制品库"
	default:
		return owner
	}
}

func infrastructurePVCLabels(owner string) map[string]string {
	return map[string]string{
		ManagedByLabel:        ManagedByValue,
		InfrastructureLabel:   owner,
		"cylism.io/component": "monitoring",
	}
}

func persistentVolumeNodeName(volume *corev1.PersistentVolume) string {
	terms := volume.Spec.NodeAffinity
	if terms == nil || terms.Required == nil {
		return ""
	}
	for _, term := range terms.Required.NodeSelectorTerms {
		for _, requirement := range term.MatchExpressions {
			if requirement.Key == corev1.LabelHostname && requirement.Operator == corev1.NodeSelectorOpIn && len(requirement.Values) == 1 {
				return requirement.Values[0]
			}
		}
	}
	return ""
}

func persistentVolumeLocalPath(volume *corev1.PersistentVolume) (string, bool) {
	if volume.Spec.HostPath != nil && strings.TrimSpace(volume.Spec.HostPath.Path) != "" {
		return volume.Spec.HostPath.Path, true
	}
	if volume.Spec.Local != nil && strings.TrimSpace(volume.Spec.Local.Path) != "" {
		return volume.Spec.Local.Path, true
	}
	return "", false
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringValue(value *storagev1.VolumeBindingMode) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func reclaimPolicyValue(value *corev1.PersistentVolumeReclaimPolicy) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
