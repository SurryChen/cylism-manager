package k8s

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestListManagedPVCsIncludesBoundNodeAndReclaimPolicy(t *testing.T) {
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "karakeep-data", Namespace: "project-knowledge", Labels: map[string]string{ManagedByLabel: ManagedByValue, EnvironmentLabel: EnvironmentLabelValue(3)}},
			Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: stringPtr("local-path"), VolumeName: "pvc-001", AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")}}},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound, Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")}},
		},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pvc-001"}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: reclaimPolicy, PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/rancher/k3s/storage/pvc-001"}}, NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"storage-node-a"}}}}}}}}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path"}, VolumeBindingMode: volumeBindingModePtr(storagev1.VolumeBindingWaitForFirstConsumer)},
	)}

	pvcs, err := client.ListManagedPVCsContext(context.Background(), "project-knowledge", 3)
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
	if info.BoundNode != "storage-node-a" || info.LocalPath != "/var/lib/rancher/k3s/storage/pvc-001" || !info.IsLocal || info.ReclaimPolicy != string(corev1.PersistentVolumeReclaimDelete) || !info.WaitForFirstConsumer {
		t.Fatalf("expected node, reclaim policy and binding mode, got %#v", info)
	}
}

func TestCreatePVCBindingPodPinsClaimToTargetNode(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset()}
	pod, err := client.CreatePVCBindingPodContext(context.Background(), "project-knowledge", "migration-1", "karakeep-data-migration-1", "storage-node-b", "registry.example.com/pause:3.10")
	if err != nil {
		t.Fatal(err)
	}
	if pod.Labels[ManagedByLabel] != ManagedByValue || pod.Spec.NodeSelector[corev1.LabelHostname] != "storage-node-b" || len(pod.Spec.Volumes) != 1 || pod.Spec.Volumes[0].PersistentVolumeClaim == nil || pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "karakeep-data-migration-1" {
		t.Fatalf("unexpected binding pod: %#v", pod)
	}
	if len(pod.Spec.Containers) != 1 || pod.Spec.Containers[0].Image != "registry.example.com/pause:3.10" {
		t.Fatalf("unexpected helper container: %#v", pod.Spec.Containers)
	}
}

func TestListManagedPVCsExcludesOtherEnvironment(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "project-knowledge", Labels: map[string]string{ManagedByLabel: ManagedByValue, EnvironmentLabel: EnvironmentLabelValue(4)}}},
	)}
	pvcs, err := client.ListManagedPVCsContext(context.Background(), "project-knowledge", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pvcs) != 0 {
		t.Fatalf("expected no PVCs from another environment, got %#v", pvcs)
	}
}

func TestListManagedPVCsIncludesIndependentClaimInSameNamespace(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "project-knowledge", Labels: map[string]string{ManagedByLabel: ManagedByValue}}},
	)}
	pvcs, err := client.ListManagedPVCsContext(context.Background(), "project-knowledge", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pvcs) != 1 || pvcs[0].EnvironmentID != 0 || !pvcs[0].Managed {
		t.Fatalf("expected independent managed PVC, got %#v", pvcs)
	}
}

func TestListPVCsIncludesExternalAndManagedClaimsAcrossNamespaces(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "external-data", Namespace: "default"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "managed-data", Namespace: "project-knowledge", Labels: map[string]string{ManagedByLabel: ManagedByValue, EnvironmentLabel: EnvironmentLabelValue(3)}}},
	)}

	pvcs, err := client.ListPVCsContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(pvcs) != 2 {
		t.Fatalf("expected all PVCs, got %#v", pvcs)
	}
	if pvcs[0].Name != "external-data" || pvcs[0].Managed || pvcs[1].Name != "managed-data" || !pvcs[1].Managed || pvcs[1].EnvironmentID != 3 {
		t.Fatalf("unexpected PVC inventory: %#v", pvcs)
	}
}

func TestListPVCsBatchesPersistentVolumeAndStorageClassLookups(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "one", Namespace: "default"}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: stringPtr("local-path"), VolumeName: "pv-one"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "two", Namespace: "default"}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: stringPtr("local-path"), VolumeName: "pv-two"}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-one"}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-two"}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path"}},
	)
	getCalls := 0
	clientset.PrependReactor("get", "persistentvolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
		getCalls++
		return false, nil, nil
	})
	clientset.PrependReactor("get", "storageclasses", func(k8stesting.Action) (bool, runtime.Object, error) {
		getCalls++
		return false, nil, nil
	})

	claims, err := (&Client{Clientset: clientset}).ListPVCsContext(context.Background(), "")
	if err != nil || len(claims) != 2 {
		t.Fatalf("expected two claims, got %#v err=%v", claims, err)
	}
	if getCalls != 0 {
		t.Fatalf("expected inventory to use list calls rather than per-PVC gets, got %d gets", getCalls)
	}
}

func TestListPVCsRejectsMissingClientOrContext(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset()}
	if _, err := client.ListPVCsContext(nil, ""); err == nil || !strings.Contains(err.Error(), "上下文") {
		t.Fatalf("expected nil context error, got %v", err)
	}
	if _, err := (&Client{}).ListPVCsContext(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "客户端未初始化") {
		t.Fatalf("expected missing client error, got %v", err)
	}
}

func TestInfrastructurePVCIsClassifiedAndProtectedFromGenericDelete(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cylism-victoria-metrics-data", Namespace: "monitoring", Labels: infrastructurePVCLabels(InfrastructureVictoriaMetrics)},
	})}
	claims, err := client.ListPVCsContext(context.Background(), "monitoring")
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].OwnerType != "infrastructure" || claims[0].OwnerName != "VictoriaMetrics" || !claims[0].ReadOnly {
		t.Fatalf("expected read-only infrastructure PVC, got %#v", claims)
	}
	if err := client.DeleteManagedPVCContext(context.Background(), "monitoring", "cylism-victoria-metrics-data", 0); err == nil || !strings.Contains(err.Error(), "基础设施组件") {
		t.Fatalf("expected protected PVC delete error, got %v", err)
	}
}

func TestRuntimePVCIsClassifiedAndProtectedFromGenericDelete(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "nanobot-data", Namespace: "cylism-assistant", Labels: map[string]string{ManagedByLabel: ManagedByValue, InfrastructureLabel: InfrastructureRuntime}},
	})}
	claims, err := client.ListPVCsContext(context.Background(), "cylism-assistant")
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].OwnerType != "infrastructure" || claims[0].OwnerName != "Agent Runtime" || !claims[0].ReadOnly {
		t.Fatalf("expected runtime PVC to be read-only infrastructure, got %#v", claims)
	}
	if err := client.DeleteManagedPVCContext(context.Background(), "cylism-assistant", "nanobot-data", 0); err == nil || !strings.Contains(err.Error(), "基础设施组件") {
		t.Fatalf("expected protected runtime PVC delete error, got %v", err)
	}
}

func TestManagedOCIRegistryPVCIsClassifiedAndProtectedFromGenericDelete(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cylism-oci-registry-data", Namespace: "cylism-system", Labels: map[string]string{ManagedByLabel: ManagedByValue}},
	})}
	claims, err := client.ListPVCsContext(context.Background(), "cylism-system")
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].OwnerType != "infrastructure" || claims[0].OwnerName != "OCI 制品库" || !claims[0].ReadOnly {
		t.Fatalf("expected managed OCI registry PVC to be read-only infrastructure, got %#v", claims)
	}
	if err := client.DeleteManagedPVCContext(context.Background(), "cylism-system", "cylism-oci-registry-data", 0); err == nil || !strings.Contains(err.Error(), "基础设施组件") {
		t.Fatalf("expected protected PVC delete error, got %v", err)
	}
}

func TestCreateManagedPVCUsesEnvironmentLabelsAndReadWriteOnce(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset()}
	created, err := client.CreateManagedPVCContext(context.Background(), "project-knowledge", 3, PersistentVolumeClaimRequest{Name: "karakeep-data", Storage: "5Gi", StorageClassName: "local-path"})
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
