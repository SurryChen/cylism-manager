package k8s

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestNamespaceMaintenanceCopiesLocalVolumeAfterSourceStops(t *testing.T) {
	ctx := t.Context()
	move := ApplicationNamespaceMove{ApplicationID: 7, Name: "service", SourceNamespace: "old", TargetNamespace: "new",
		SourceProjectID: 1, SourceEnvironmentID: 2, TargetProjectID: 3, TargetEnvironmentID: 4, SourceSequence: 2}
	workloadLabels := map[string]string{ManagedByLabel: ManagedByValue, EnvironmentLabel: "environment-2", "app.kubernetes.io/part-of": "project-1", "app": move.Name, "cylism.io/release": "2"}
	replicas, yes := int32(1), true
	node, class := "node1", "local-path"
	digest := "registry.example.com/service@sha256:" + strings.Repeat("a", 64)
	selector := map[string]string{"app": move.Name}
	source := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "old", Name: move.Name, UID: "source", Labels: workloadLabels},
		Spec: appsv1.StatefulSetSpec{ServiceName: move.Name, Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: workloadLabels},
				Spec: corev1.PodSpec{HostNetwork: true, Containers: []corev1.Container{{Name: move.Name, Image: "registry.example.com/service:latest",
					EnvFrom: []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-secret"}}}}}},
					Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}}},
				}}}, Status: appsv1.StatefulSetStatus{ReadyReplicas: 1}}
	newPod := func(namespace string, uid types.UID, image string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "service-0", UID: types.UID(namespace + "-pod"), Labels: workloadLabels,
			OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: move.Name, UID: uid, Controller: &yes}}},
			Spec:   corev1.PodSpec{NodeName: node, Volumes: source.Spec.Template.Spec.Volumes},
			Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: move.Name, ImageID: image, Ready: true}}}}
	}
	claim := func(namespace, volume string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "data", UID: types.UID(namespace + "-claim"),
			Labels: map[string]string{ManagedByLabel: ManagedByValue}, Annotations: map[string]string{applicationMoveAnnotation: "1"}},
			Spec: corev1.PersistentVolumeClaimSpec{VolumeName: volume, StorageClassName: &class, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}
	}
	volume := func(claim *corev1.PersistentVolumeClaim, path string) *corev1.PersistentVolume {
		return &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: claim.Spec.VolumeName, Annotations: map[string]string{"pv.kubernetes.io/provisioned-by": "rancher.io/local-path"}},
			Spec: corev1.PersistentVolumeSpec{ClaimRef: &corev1.ObjectReference{Namespace: claim.Namespace, Name: claim.Name, UID: claim.UID},
				PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path}},
				NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{
					{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{node}},
				}}}}},
			}}
	}
	oldClaim, newClaim := claim("old", "old-volume"), claim("new", "new-volume")
	oldClaim.Labels = nil
	binding := storagev1.VolumeBindingWaitForFirstConsumer
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "old", Name: move.Name, UID: "source-service", Labels: workloadLabels},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: selector, Ports: []corev1.ServicePort{{Port: 443, Protocol: corev1.ProtocolUDP}}}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "old", Name: "app-secret", UID: "source-secret"},
		Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"key": []byte("test-value")}}
	clientset := fake.NewSimpleClientset(source, newPod("old", source.UID, digest), service, secret, oldClaim, volume(oldClaim, "/storage/pvc-old-claim"),
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: node, Labels: map[string]string{corev1.LabelHostname: node}},
			Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: class}, Provisioner: "rancher.io/local-path", VolumeBindingMode: &binding})
	client := &Client{Clientset: clientset}

	plan, err := client.PlanApplicationNamespaceMaintenance(ctx, move, "statefulset")
	if err != nil || plan.Node != node || len(plan.Claims) != 1 || !plan.ManualValidation {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	if err := client.CheckApplicationNamespaceMaintenance(ctx, move, plan, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims("new").Create(ctx, newClaim, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().PersistentVolumes().Create(ctx, volume(newClaim, "/storage/new"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	checks, copies := 0, 0
	copyVolume := func(ctx context.Context, from, to string, copy bool) error {
		if from != "/storage/pvc-old-claim" || to != "/storage/new" {
			t.Fatalf("wrong copy paths %q -> %q", from, to)
		}
		if copy {
			current, err := clientset.AppsV1().StatefulSets("old").Get(ctx, move.Name, metav1.GetOptions{})
			if err != nil || *current.Spec.Replicas != 0 {
				t.Fatalf("source was not stopped before copying: %v", err)
			}
			copies++
		} else {
			checks++
		}
		return nil
	}
	if err := client.PrepareApplicationNamespaceMaintenance(ctx, move, plan, "1", copyVolume); err != nil {
		t.Fatal(err)
	}
	target, err := clientset.AppsV1().StatefulSets("new").Get(ctx, move.Name, metav1.GetOptions{})
	if err != nil || *target.Spec.Replicas != 0 || target.Spec.Template.Spec.Containers[0].Image != digest {
		t.Fatalf("preparation started the target or changed the image: %v", err)
	}
	copiedSecret, err := clientset.CoreV1().Secrets("new").Get(ctx, secret.Name, metav1.GetOptions{})
	if err != nil || string(copiedSecret.Data["key"]) != "test-value" {
		t.Fatalf("target Secret missing: %v", err)
	}
	clientset.PrependReactor("patch", "statefulsets", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == "old" {
			err := clientset.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), "old", "service-0")
			return err != nil, nil, err
		}
		return false, nil, nil
	})
	if err := client.CopyApplicationNamespaceMaintenance(ctx, move, plan, "1", nil, copyVolume); err != nil {
		t.Fatal(err)
	}
	if checks != 2 || copies != 1 {
		t.Fatalf("directory checks = %d, copies = %d", checks, copies)
	}
	if _, err := client.RunApplicationNamespaceMaintenanceTarget(ctx, move, plan, "1", true); err != nil {
		t.Fatal(err)
	}
	target, _ = clientset.AppsV1().StatefulSets("new").Get(ctx, move.Name, metav1.GetOptions{})
	target.Status.ReadyReplicas = 1
	if _, err := clientset.AppsV1().StatefulSets("new").UpdateStatus(ctx, target, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	pod := newPod("new", target.UID, digest)
	if _, err := clientset.CoreV1().Pods("new").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	slice := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Namespace: "new", Name: move.Name,
		Labels: map[string]string{discoveryv1.LabelServiceName: move.Name}, OwnerReferences: []metav1.OwnerReference{{Kind: "Service", Controller: &yes}}},
		Endpoints: []discoveryv1.Endpoint{{Conditions: discoveryv1.EndpointConditions{Ready: &yes}, TargetRef: &corev1.ObjectReference{Kind: "Pod", Namespace: "new", UID: pod.UID}}}}
	if _, err := clientset.DiscoveryV1().EndpointSlices("new").Create(ctx, slice, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if ready, err := client.RunApplicationNamespaceMaintenanceTarget(ctx, move, plan, "1", false); err != nil || !ready {
		t.Fatalf("target ready = %v, %v", ready, err)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims("old").Get(ctx, oldClaim.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("source volume was removed: %v", err)
	}
}

func TestNamespaceMaintenanceRouteCutoverCanResume(t *testing.T) {
	route := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Namespace: "old", Name: "service", UID: "original",
		Labels: map[string]string{ManagedByLabel: ManagedByValue}}}
	client := &Client{Clientset: fake.NewSimpleClientset(route)}
	ctx := t.Context()
	plan := &ApplicationNamespaceMaintenancePlan{Ingress: string(namespaceMoveJSON(t, route))}
	move := ApplicationNamespaceMove{SourceNamespace: "old", TargetNamespace: "new", Name: "service"}
	if err := client.SwitchApplicationNamespaceMaintenanceRoutes(ctx, move, plan, "1", nil, true); err != nil {
		t.Fatal(err)
	}
	ingresses := client.Clientset.NetworkingV1().Ingresses("new")
	target, err := ingresses.Get(ctx, route.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	class := "traefik"
	target.Spec.IngressClassName = &class
	if _, err := ingresses.Update(ctx, target, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := client.SwitchApplicationNamespaceMaintenanceRoutes(ctx, move, plan, "1", nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.NetworkingV1().Ingresses("old").Get(ctx, route.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("source route still exists: %v", err)
	}
	if err := client.SwitchApplicationNamespaceMaintenanceRoutes(ctx, move, plan, "2", nil, true); err == nil {
		t.Fatal("accepted an Ingress from another migration")
	}
}

func namespaceMoveJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
