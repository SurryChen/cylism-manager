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
	_ = client.CreateConfigMap
	_ = client.UpdateConfigMap
	_ = client.DeleteConfigMap
	_ = client.CreateOpaqueSecret
	_ = client.UpdateOpaqueSecret
	_ = client.DeleteOpaqueSecret
}

func TestConfigMapMutationLifecycle(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	client.Clientset = k8sfake.NewSimpleClientset()
	created, err := client.CreateConfigMap(ConfigMapMutation{Namespace: "apps", Name: "app-config", Data: map[string]string{"config.yaml": "version: 1"}})
	if err != nil || created.Data["config.yaml"] != "version: 1" {
		t.Fatalf("create configmap: result=%#v err=%v", created, err)
	}
	updated, err := client.UpdateConfigMap(ConfigMapMutation{Namespace: "apps", Name: "app-config", Data: map[string]string{"config.yaml": "version: 2"}})
	if err != nil || updated.Data["config.yaml"] != "version: 2" {
		t.Fatalf("update configmap: result=%#v err=%v", updated, err)
	}
	if err := client.DeleteConfigMap("apps", "app-config"); err != nil {
		t.Fatalf("delete configmap: %v", err)
	}
}

func TestOpaqueSecretMutationPreservesBlankExistingValues(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	client.Clientset = k8sfake.NewSimpleClientset()
	created, err := client.CreateOpaqueSecret(OpaqueSecretMutation{Namespace: "apps", Name: "app-secret", Data: map[string]string{"token": "first"}})
	if err != nil || created.Type != string(corev1.SecretTypeOpaque) {
		t.Fatalf("create opaque secret: result=%#v err=%v", created, err)
	}
	updated, err := client.UpdateOpaqueSecret(OpaqueSecretMutation{Namespace: "apps", Name: "app-secret", Data: map[string]string{"token": "", "region": "cn"}})
	if err != nil || updated.KeysCount != 2 {
		t.Fatalf("update opaque secret: result=%#v err=%v", updated, err)
	}
	secret, err := client.Clientset.CoreV1().Secrets("apps").Get(client.Ctx(), "app-secret", metav1.GetOptions{})
	if err != nil || string(secret.Data["token"]) != "first" || string(secret.Data["region"]) != "cn" {
		t.Fatalf("secret values were not preserved: secret=%#v err=%v", secret, err)
	}
	if err := client.DeleteOpaqueSecret("apps", "app-secret"); err != nil {
		t.Fatalf("delete opaque secret: %v", err)
	}
}

func TestOpaqueSecretMutationRejectsManagedSecretTypes(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	client.Clientset = k8sfake.NewSimpleClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "tls", Namespace: "apps"}, Type: corev1.SecretTypeTLS})
	if _, err := client.UpdateOpaqueSecret(OpaqueSecretMutation{Namespace: "apps", Name: "tls", Data: map[string]string{"tls.crt": "new"}}); err == nil {
		t.Fatal("expected TLS Secret update to be rejected")
	}
	if err := client.DeleteOpaqueSecret("apps", "tls"); err == nil {
		t.Fatal("expected TLS Secret delete to be rejected")
	}
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
