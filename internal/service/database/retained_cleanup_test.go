package database

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestRetainedCleanupOptionsAreIndependentAndScoped(t *testing.T) {
	for _, deleteClaims := range []bool{false, true} {
		for _, deleteSecrets := range []bool{false, true} {
			name := "keep"
			if deleteClaims {
				name += "-delete-claims"
			}
			if deleteSecrets {
				name += "-delete-secrets"
			}
			t.Run(name, func(t *testing.T) {
				client, item := lifecycleFixture(t)
				ctx := context.Background()
				for i := 0; i < 4; i++ {
					if _, _, err := RetireClusterStep(ctx, client, item); err != nil {
						t.Fatal(err)
					}
				}
				root := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-root", Namespace: item.Namespace, Labels: map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue}}, Data: map[string][]byte{"rootPassword": []byte("never-return-this")}}
				internal := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-privsecrets", Namespace: item.Namespace, OwnerReferences: []metav1.OwnerReference{{Kind: "InnoDBCluster", APIVersion: "mysql.oracle.com/v2", Name: item.ResourceName}}}}
				unrelated := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-app-password", Namespace: item.Namespace, Labels: map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue}}}
				for _, secret := range []*corev1.Secret{root, internal, unrelated} {
					if _, err := client.Clientset.CoreV1().Secrets(item.Namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
						t.Fatal(err)
					}
				}
				state, err := InspectClusterCleanup(ctx, client, item)
				if err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(state)
				if len(state.Secrets) != 2 || strings.Contains(string(encoded), "never-return-this") {
					t.Fatalf("secret inventory exposes values or misidentifies ownership: %s", encoded)
				}
				done, _, err := CleanupRetainedClusterResources(ctx, client, item, deleteClaims, deleteSecrets)
				if err != nil || !done {
					t.Fatalf("done=%v err=%v", done, err)
				}
				_, err = client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace).Get(ctx, "datadir-"+item.ResourceName+"-0", metav1.GetOptions{})
				if apierrors.IsNotFound(err) != deleteClaims {
					t.Fatalf("claim deletion=%v err=%v", deleteClaims, err)
				}
				for _, secret := range []*corev1.Secret{root, internal} {
					_, err = client.Clientset.CoreV1().Secrets(item.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
					if apierrors.IsNotFound(err) != deleteSecrets {
						t.Fatalf("secret deletion=%v err=%v", deleteSecrets, err)
					}
				}
				if _, err = client.Clientset.CoreV1().Secrets(item.Namespace).Get(ctx, unrelated.Name, metav1.GetOptions{}); err != nil {
					t.Fatal("unrelated Secret removed")
				}
				if done, _, err = CleanupRetainedClusterResources(ctx, client, item, deleteClaims, deleteSecrets); err != nil || !done {
					t.Fatalf("cleanup not idempotent: done=%v err=%v", done, err)
				}
			})
		}
	}
}
func TestRetainedCleanupWaitsForPVCDeletionBeforeRemovingCredentials(t *testing.T) {
	client, item := lifecycleFixture(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if _, _, err := RetireClusterStep(ctx, client, item); err != nil {
			t.Fatal(err)
		}
	}
	root := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-root", Namespace: item.Namespace, Labels: map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue}}}
	client.Clientset.CoreV1().Secrets(item.Namespace).Create(ctx, root, metav1.CreateOptions{})
	client.Clientset.(*k8sfake.Clientset).PrependReactor("delete", "persistentvolumeclaims", func(k8stesting.Action) (bool, runtime.Object, error) { return true, nil, nil })
	done, progress, err := CleanupRetainedClusterResources(ctx, client, item, true, true)
	if err != nil || done || !strings.Contains(progress, "等待") {
		t.Fatalf("done=%v progress=%s err=%v", done, progress, err)
	}
	if _, err := client.Clientset.CoreV1().Secrets(item.Namespace).Get(ctx, root.Name, metav1.GetOptions{}); err != nil {
		t.Fatal("credential deleted before data cleanup completed")
	}
}
func TestRetainedCleanupRefusesLiveCluster(t *testing.T) {
	client, item := lifecycleFixture(t)
	client.Clientset.(*k8sfake.Clientset).ClearActions()
	if _, _, err := CleanupRetainedClusterResources(context.Background(), client, item, true, true); err == nil {
		t.Fatal("live cluster cleanup allowed")
	}
	for _, action := range client.Clientset.(*k8sfake.Clientset).Actions() {
		if action.GetVerb() == "delete" {
			t.Fatal("resource deleted before cluster removal")
		}
	}
}
