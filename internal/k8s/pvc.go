package k8s

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	ManagedByLabel   = "app.kubernetes.io/managed-by"
	ManagedByValue   = "cylism-manager"
	EnvironmentLabel = "cylism.io/environment"
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
	Phase                string   `json:"phase"`
	Storage              string   `json:"storage"`
	StorageClassName     string   `json:"storage_class_name,omitempty"`
	VolumeName           string   `json:"volume_name,omitempty"`
	AccessModes          []string `json:"access_modes"`
	BoundNode            string   `json:"bound_node,omitempty"`
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
	claims, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(c.Ctx(), metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s,%s=%s", ManagedByLabel, ManagedByValue, EnvironmentLabel, EnvironmentLabelValue(environmentID))})
	if err != nil {
		return nil, fmt.Errorf("list persistentvolumeclaims: %w", err)
	}
	result := make([]PersistentVolumeClaimInfo, 0, len(claims.Items))
	for index := range claims.Items {
		info, err := c.pvcInfo(&claims.Items[index])
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
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
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{ManagedByLabel: ManagedByValue, EnvironmentLabel: EnvironmentLabelValue(environmentID)}},
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

func (c *Client) DeleteManagedPVC(namespace, name string, environmentID uint) error {
	claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get persistentvolumeclaim %s/%s: %w", namespace, name, err)
	}
	if !isManagedPVC(claim, environmentID) {
		return fmt.Errorf("PVC %q 不属于当前环境或未由平台管理", name)
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
	info := PersistentVolumeClaimInfo{
		Name:              claim.Name,
		Namespace:         claim.Namespace,
		Phase:             string(claim.Status.Phase),
		StorageClassName:  valueOrEmpty(claim.Spec.StorageClassName),
		VolumeName:        claim.Spec.VolumeName,
		CreationTimestamp: claim.CreationTimestamp.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if storage := claim.Status.Capacity.Storage(); storage != nil {
		info.Storage = storage.String()
	} else if storage := claim.Spec.Resources.Requests.Storage(); storage != nil {
		info.Storage = storage.String()
	}
	for _, mode := range claim.Spec.AccessModes {
		info.AccessModes = append(info.AccessModes, string(mode))
	}
	if info.StorageClassName != "" {
		storageClass, err := c.Clientset.StorageV1().StorageClasses().Get(c.Ctx(), info.StorageClassName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return PersistentVolumeClaimInfo{}, fmt.Errorf("get storageclass %s: %w", info.StorageClassName, err)
		}
		if storageClass != nil && storageClass.VolumeBindingMode != nil {
			info.WaitForFirstConsumer = *storageClass.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
		}
	}
	if info.VolumeName == "" {
		return info, nil
	}
	pv, err := c.Clientset.CoreV1().PersistentVolumes().Get(c.Ctx(), info.VolumeName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return info, nil
		}
		return PersistentVolumeClaimInfo{}, fmt.Errorf("get persistentvolume %s: %w", info.VolumeName, err)
	}
	info.ReclaimPolicy = string(pv.Spec.PersistentVolumeReclaimPolicy)
	info.BoundNode = persistentVolumeNodeName(pv)
	return info, nil
}

func isManagedPVC(claim *corev1.PersistentVolumeClaim, environmentID uint) bool {
	return claim.Labels[ManagedByLabel] == ManagedByValue && claim.Labels[EnvironmentLabel] == EnvironmentLabelValue(environmentID)
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
