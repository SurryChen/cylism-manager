package store

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestManagedOCIRegistryPersistsOwnedRegistryRecordsWithoutCredential(t *testing.T) {
	store, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	managed := &model.ManagedOCIRegistry{Name: "内网制品库", Namespace: "cylism-system", ResourceName: "cylism-registry", Endpoint: "registry.internal:5443", RegistryImage: "registry:2.8", DataNode: "node-a", StorageClassName: "local-path", PVCName: "cylism-registry-data", StorageSize: "100Gi", CPURequest: "100m", CPULimit: "500m", MemoryRequest: "256Mi", MemoryLimit: "1Gi", CertificateName: "registry-cert", PullUsername: "cylism-pull", EncryptedCredential: "encrypted", CreatedBy: 1}
	image := &model.ImageRegistry{Name: "内网制品库", Endpoint: managed.Endpoint, AuthType: "basic", Username: managed.PullUsername, Credential: "encrypted", CreatedBy: 1}
	mirror := &model.NodeRegistryMirror{Name: "内网制品库", Registry: managed.Endpoint, Endpoints: `["https://registry.internal:5443"]`, Username: managed.PullUsername, Credential: "encrypted", Enabled: true, CreatedBy: 1}
	if err := store.CreateManagedOCIRegistry(managed, image, mirror, nil); err != nil {
		t.Fatal(err)
	}
	if managed.ID == 0 || managed.ImageRegistryID == nil || managed.NodeRegistryMirrorID == nil {
		t.Fatalf("managed ownership IDs were not saved: %#v", managed)
	}
	loaded, err := store.GetManagedOCIRegistry(managed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.CredentialConfigured || loaded.EncryptedCredential != "encrypted" {
		t.Fatalf("credential state mismatch: %#v", loaded)
	}
	if loaded.PVCName != "cylism-registry-data" || loaded.StorageSize != "100Gi" || loaded.CPURequest != "100m" || loaded.MemoryLimit != "1Gi" || loaded.CertificateName != "registry-cert" {
		t.Fatalf("PVC or resource configuration mismatch: %#v", loaded)
	}
	if image.ManagedRegistryID == nil || *image.ManagedRegistryID != managed.ID || mirror.ManagedRegistryID == nil || *mirror.ManagedRegistryID != managed.ID {
		t.Fatalf("associated ownership IDs missing: %#v %#v", image, mirror)
	}
}
