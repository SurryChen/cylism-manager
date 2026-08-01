package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestListManagedPVCsIncludesBoundNodeAndReclaimPolicy(t *testing.T) {
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "karakeep-data", Namespace: "project-knowledge", Labels: map[string]string{ManagedByLabel: ManagedByValue, EnvironmentLabel: EnvironmentLabelValue(3)}},
			Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: stringPtr("local-path"), VolumeName: "pvc-001", AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")}}},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound, Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")}},
		},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pvc-001"}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: reclaimPolicy, NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"storage-node-a"}}}}}}}}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path"}, VolumeBindingMode: volumeBindingModePtr(storagev1.VolumeBindingWaitForFirstConsumer)},
	)}

	pvcs, err := client.ListManagedPVCs("project-knowledge", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pvcs) != 1 {
		t.Fatalf("expected one PVC, got %#v", pvcs)
	}
	info := pvcs[0]
	if info.Name != "karakeep-data" || info.Phase != string(corev1.ClaimBound) || info.Storage != "5Gi" || info.StorageClassName != "local-path" {
		t.Fatalf("unexpected PVC info: %#v", info)
	}
	if info.BoundNode != "storage-node-a" || info.ReclaimPolicy != string(corev1.PersistentVolumeReclaimDelete) || !info.WaitForFirstConsumer {
		t.Fatalf("expected node, reclaim policy and binding mode, got %#v", info)
	}
}

func TestListManagedPVCsExcludesOtherEnvironment(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "project-knowledge", Labels: map[string]string{ManagedByLabel: ManagedByValue, EnvironmentLabel: EnvironmentLabelValue(4)}}},
	)}
	pvcs, err := client.ListManagedPVCs("project-knowledge", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pvcs) != 0 {
		t.Fatalf("expected no PVCs from another environment, got %#v", pvcs)
	}
}

func TestCreateManagedPVCUsesEnvironmentLabelsAndReadWriteOnce(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset()}
	created, err := client.CreateManagedPVC("project-knowledge", 3, PersistentVolumeClaimRequest{Name: "karakeep-data", Storage: "5Gi", StorageClassName: "local-path"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Labels[ManagedByLabel] != ManagedByValue || created.Labels[EnvironmentLabel] != EnvironmentLabelValue(3) {
		t.Fatalf("missing managed labels: %#v", created.Labels)
	}
	if len(created.Spec.AccessModes) != 1 || created.Spec.AccessModes[0] != corev1.ReadWriteOnce || created.Spec.Resources.Requests.Storage().String() != "5Gi" {
		t.Fatalf("unexpected PVC spec: %#v", created.Spec)
	}
}

func stringPtr(value string) *string { return &value }

func volumeBindingModePtr(value storagev1.VolumeBindingMode) *storagev1.VolumeBindingMode {
	return &value
}
