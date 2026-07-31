package k8s

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestConfigMapInfo_FieldsComplete(t *testing.T) {
	c := ConfigMapInfo{
		Name:      "app-config",
		Namespace: "default",
		Keys:      []string{"PORT", "LOG_LEVEL"},
		KeysCount: 2,
		UsedBy:    []WorkloadRef{{Kind: "Deployment", Name: "web-app", RefType: "env"}},
		Age:       "12d",
	}
	if c.KeysCount != 2 {
		t.Errorf("expected 2 keys, got %d", c.KeysCount)
	}
}

func TestSecretInfo_FieldsComplete(t *testing.T) {
	s := SecretInfo{
		Name:      "db-password",
		Namespace: "default",
		Type:      "Opaque",
		Keys:      []string{"password"},
		KeysCount: 1,
		UsedBy:    []WorkloadRef{{Kind: "Deployment", Name: "api-server", RefType: "volume"}},
		Age:       "5d",
	}
	if s.Type != "Opaque" {
		t.Errorf("expected Opaque type, got %s", s.Type)
	}
}

func TestSecretDetail_FieldsComplete(t *testing.T) {
	s := SecretDetail{
		Name:      "tls-cert",
		Namespace: "default",
		Type:      "kubernetes.io/tls",
		Data:      map[string]string{"tls.crt": "base64encoded...", "tls.key": "base64encoded..."},
		UsedBy:    []WorkloadRef{},
		Age:       "30d",
	}
	if len(s.Data) != 2 {
		t.Errorf("expected 2 data keys, got %d", len(s.Data))
	}
}

func TestConfigMethods_Exist(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	_ = client.ListConfigMaps
	_ = client.GetConfigMap
	_ = client.ListSecrets
	_ = client.GetSecret
}

func TestListConfigMapsBatchesWorkloadReferenceLookupsByNamespace(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	clientset := newConfigReferenceTestClientset()
	client.Clientset = clientset
	workloadLists := countWorkloadListRequests(clientset)

	items, err := client.ListConfigMaps("")
	if err != nil {
		t.Fatal(err)
	}
	if *workloadLists != 6 {
		t.Fatalf("expected one list for each workload type in two namespaces, got %d", *workloadLists)
	}
	if !allConfigMapItemsReferenced(items) {
		t.Fatalf("expected references for all ConfigMaps, got %#v", items)
	}
}

func TestListSecretsBatchesWorkloadReferenceLookupsByNamespace(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	clientset := newConfigReferenceTestClientset()
	client.Clientset = clientset
	workloadLists := countWorkloadListRequests(clientset)

	items, err := client.ListSecrets("")
	if err != nil {
		t.Fatal(err)
	}
	if *workloadLists != 6 {
		t.Fatalf("expected one list for each workload type in two namespaces, got %d", *workloadLists)
	}
	if !allSecretItemsReferenced(items) {
		t.Fatalf("expected references for all Secrets, got %#v", items)
	}
}

func newConfigReferenceTestClientset() *k8sfake.Clientset {
	teamContainer := corev1.Container{
		Name: "app",
		EnvFrom: []corev1.EnvFromSource{
			{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "team-config-env"}}},
			{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "team-secret-env"}}},
		},
	}
	monitoringContainer := corev1.Container{
		Name: "agent",
		EnvFrom: []corev1.EnvFromSource{
			{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "monitoring-config"}}},
			{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "monitoring-secret"}}},
		},
	}
	return k8sfake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "team-config-env", Namespace: "team"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "team-config-volume", Namespace: "team"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "monitoring-config", Namespace: "monitoring"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "team-secret-env", Namespace: "team"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "team-secret-volume", Namespace: "team"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "monitoring-secret", Namespace: "monitoring"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "team"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{teamContainer}, Volumes: []corev1.Volume{
			{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "team-config-volume"}}}},
			{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "team-secret-volume"}}},
		}}}}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "monitoring"}, Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{monitoringContainer}}}}},
	)
}

func countWorkloadListRequests(clientset *k8sfake.Clientset) *int {
	count := 0
	countList := func(k8stesting.Action) (bool, runtime.Object, error) {
		count++
		return false, nil, nil
	}
	clientset.PrependReactor("list", "deployments", countList)
	clientset.PrependReactor("list", "statefulsets", countList)
	clientset.PrependReactor("list", "daemonsets", countList)
	return &count
}

func allConfigMapItemsReferenced(items []ConfigMapInfo) bool {
	if len(items) != 3 {
		return false
	}
	for _, item := range items {
		if len(item.UsedBy) == 0 {
			return false
		}
	}
	return true
}

func allSecretItemsReferenced(items []SecretInfo) bool {
	if len(items) != 3 {
		return false
	}
	for _, item := range items {
		if len(item.UsedBy) == 0 {
			return false
		}
	}
	return true
}
