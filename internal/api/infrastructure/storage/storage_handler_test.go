package storage

import (
	"reflect"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/store"
	corev1 "k8s.io/api/core/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNewPVCAdaptersReturnOneSharedClientAdapter(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	pvc, migration, workloads := NewPVCAdapters(client)
	if pvc == nil || migration == nil || workloads == nil {
		t.Fatal("expected all PVC ports to be composed")
	}
	if reflect.ValueOf(pvc).Pointer() != reflect.ValueOf(migration).Pointer() || reflect.ValueOf(pvc).Pointer() != reflect.ValueOf(workloads).Pointer() {
		t.Fatal("PVC ports should share one underlying adapter")
	}
}

func TestNewPVCAdaptersReturnNilForUnavailableKubernetes(t *testing.T) {
	for _, client := range []*k8sclient.Client{nil, {}, {Clientset: nil}} {
		pvc, migration, workloads := NewPVCAdapters(client)
		if pvc != nil || migration != nil || workloads != nil {
			t.Fatalf("expected nil ports for unavailable client, got %T %T %T", pvc, migration, workloads)
		}
	}
}

func TestNewStorageHandlerWithDependenciesCopiesEncryptionKey(t *testing.T) {
	service := storageservice.NewService(nil, nil)
	key := []byte("storage-key")
	handler := NewStorageHandlerWithDependencies(service, nil, key, nil, nil, nil)
	key[0] = 'X'
	if string(handler.encKey) != "storage-key" {
		t.Fatalf("handler retained mutable encryption key: %q", handler.encKey)
	}
	if handler.Service != service {
		t.Fatal("handler did not retain composed storage service")
	}
}

func TestConfigureStorageExecutorIsSafeWithoutService(t *testing.T) {
	(&StorageHandler{}).ConfigureStorageExecutor()
}

func TestStorageHandlerAcceptsInjectedPVCPorts(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{})}
	pvc, migration, workloads := NewPVCAdapters(client)
	service := storageservice.NewService(pvc, nil)
	handler := NewStorageHandlerWithDependencies(service, nil, nil, pvc, migration, workloads)
	if handler.pvc != pvc || handler.migration != migration || handler.workloads != workloads {
		t.Fatal("handler did not retain injected PVC ports")
	}
}

func TestStorageHandlerCanBeConstructedWithStore(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewStorageHandlerWithDependencies(nil, st, nil, nil, nil, nil)
	if handler.store != st {
		t.Fatal("handler did not retain repository")
	}
}
