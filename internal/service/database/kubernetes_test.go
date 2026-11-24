package database

import (
	"context"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestApplyAndDeleteRetainsClaim(t *testing.T) {
	ctx := context.Background()
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	instance := &model.DatabaseInstance{ID: 7, StorageSize: "10Gi", DatabaseName: "app_db", Username: "app_user"}
	if err := Apply(ctx, client, instance, "root-secret", "user-secret"); err != nil {
		t.Fatal(err)
	}
	set, err := client.Clientset.AppsV1().StatefulSets(Namespace).Get(ctx, ResourceName(7), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Spec.VolumeClaimTemplates) != 1 || set.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests.Storage().String() != "10Gi" {
		t.Fatalf("missing persistent storage: %+v", set.Spec.VolumeClaimTemplates)
	}
	if set.Spec.Template.Spec.Containers[0].Env[0].ValueFrom.SecretKeyRef == nil {
		t.Fatal("root password must come from a Secret")
	}
	secret, err := client.Clientset.CoreV1().Secrets(Namespace).Get(ctx, ResourceName(7), metav1.GetOptions{})
	if err != nil || secret.StringData["root-password"] != "root-secret" {
		t.Fatalf("secret missing: %v", err)
	}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: PVCName(7), Namespace: Namespace}}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(Namespace).Create(ctx, claim, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, client, instance); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(Namespace).Get(ctx, PVCName(7), metav1.GetOptions{}); err != nil {
		t.Fatalf("PVC should remain after deleting instance: %v", err)
	}
}

func TestStandaloneEmptyMySQLUsesPlacementLimitsAndReplicationParameters(t *testing.T) {
	ctx := context.Background()
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	instance := &model.DatabaseInstance{ID: 19, StorageSize: "10Gi", NodeName: "ubuntudev", CPULimit: "1", MemoryLimit: "1Gi"}
	if err := Apply(ctx, client, instance, "generated-root", ""); err != nil {
		t.Fatal(err)
	}
	set, err := client.Clientset.AppsV1().StatefulSets(Namespace).Get(ctx, ResourceName(instance.ID), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pod := set.Spec.Template.Spec
	container := pod.Containers[0]
	if pod.NodeSelector[corev1.LabelHostname] != "ubuntudev" || container.Resources.Limits.Cpu().String() != "1" || container.Resources.Limits.Memory().String() != "1Gi" {
		t.Fatal("placement or resource budget lost")
	}
	for _, env := range container.Env {
		if env.Name == "MYSQL_DATABASE" || env.Name == "MYSQL_USER" || env.Name == "MYSQL_PASSWORD" {
			t.Fatal("empty deployment invented business database credentials")
		}
	}
	args := strings.Join(container.Args, " ")
	for _, required := range []string{"--server-id=10019", "--gtid-mode=ON", "--log-bin=mysql-bin", "--innodb-buffer-pool-size="} {
		if !strings.Contains(args, required) {
			t.Fatalf("missing argument %s", required)
		}
	}
	svc, err := client.Clientset.CoreV1().Services(Namespace).Get(ctx, ResourceName(instance.ID), metav1.GetOptions{})
	if err != nil || svc.Spec.Type == corev1.ServiceTypeNodePort || svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		t.Fatal("management connection must stay inside cluster Service")
	}
}

func TestRedeployPreservesStartupReplicationFilters(t *testing.T) {
	ctx := context.Background()
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	instance := &model.DatabaseInstance{ID: 23, StorageSize: "10Gi"}
	if err := Apply(ctx, client, instance, "root-secret", ""); err != nil {
		t.Fatal(err)
	}
	sets := client.Clientset.AppsV1().StatefulSets(Namespace)
	set, _ := sets.Get(ctx, ResourceName(23), metav1.GetOptions{})
	set.Spec.Template.Spec.Containers[0].Args = append(set.Spec.Template.Spec.Containers[0].Args, "--replicate-do-db=business", "--replicate-wild-do-table=business.%", "--max-connections=9000")
	if _, err := sets.Update(ctx, set, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, client, instance, "root-secret", ""); err != nil {
		t.Fatal(err)
	}
	set, _ = sets.Get(ctx, ResourceName(23), metav1.GetOptions{})
	args := strings.Join(set.Spec.Template.Spec.Containers[0].Args, " ")
	if !strings.Contains(args, "--replicate-do-db=business") || !strings.Contains(args, "--replicate-wild-do-table=business.%") || strings.Contains(args, "--max-connections=9000") {
		t.Fatalf("replication scope or generated resource tuning lost: %s", args)
	}
}
