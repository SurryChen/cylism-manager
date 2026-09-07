package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestManagedRegistryReconcilerResolvePVCUsesExistingLocalClaim(t *testing.T) {
	mode := storagev1.VolumeBindingWaitForFirstConsumer
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path"}, VolumeBindingMode: &mode},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-data", Namespace: "cylism-system"},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: stringPtr("local-path"),
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				}},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
		},
	)}
	reconciler := NewManagedRegistryReconciler(client)
	registry := &model.ManagedOCIRegistry{Namespace: "cylism-system", ResourceName: "cylism-oci-registry", PVCName: "registry-data", DataNode: "node-a"}
	if err := reconciler.ResolvePVC(context.Background(), registry); err != nil {
		t.Fatalf("ResolvePVC returned error: %v", err)
	}
	if registry.StorageClassName != "local-path" || registry.StorageSize != "10Gi" {
		t.Fatalf("PVC metadata was not resolved: %#v", registry)
	}
}

func TestManagedRegistryReconcilerRejectsPVCAlreadyUsedByWorkload(t *testing.T) {
	mode := storagev1.VolumeBindingWaitForFirstConsumer
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path"}, VolumeBindingMode: &mode},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-data", Namespace: "cylism-system"},
			Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: stringPtr("local-path"), AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}}},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "other-workload", Namespace: "cylism-system"},
			Spec:       appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "registry-data"}}}}}}},
		},
	)}
	reconciler := NewManagedRegistryReconciler(client)
	registry := &model.ManagedOCIRegistry{Namespace: "cylism-system", ResourceName: "cylism-oci-registry", PVCName: "registry-data", DataNode: "node-a"}
	if err := reconciler.ResolvePVC(context.Background(), registry); err == nil || !strings.Contains(err.Error(), "Deployment") {
		t.Fatalf("expected workload reference rejection, got %v", err)
	}
	if err := reconciler.EnsureStorageClass(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("expected missing StorageClass error, got %v", err)
	}
}

func TestManagedRegistryReconcilerListsPVCUsedByOwnedRegistry(t *testing.T) {
	mode := storagev1.VolumeBindingWaitForFirstConsumer
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path"}, VolumeBindingMode: &mode},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-data", Namespace: "cylism-system"},
			Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: stringPtr("local-path"), AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}}},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cylism-oci-registry", Namespace: "cylism-system", Labels: map[string]string{"cylism.io/managed-registry": "true"}},
			Spec:       appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "registry-data"}}}}}}},
		},
	)}
	options, err := NewManagedRegistryReconciler(client).ListEligiblePVCs(context.Background(), "cylism-system")
	if err != nil {
		t.Fatalf("ListEligiblePVCs returned error: %v", err)
	}
	if len(options) != 1 || options[0].Name != "registry-data" {
		t.Fatalf("expected Registry-owned PVC to remain selectable, got %#v", options)
	}
}
