package database

import (
	"context"
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
