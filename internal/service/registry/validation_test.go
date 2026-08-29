package registry

import (
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/cylism/cylism-manager/internal/store"
)

func TestNormalizeEndpointAcceptsHostnameAndPort(t *testing.T) {
	endpoint, host, err := NormalizeEndpoint(" Registry.Example.com:5443 ")
	if err != nil {
		t.Fatalf("NormalizeEndpoint returned error: %v", err)
	}
	if endpoint != "registry.example.com:5443" || host != "registry.example.com" {
		t.Fatalf("unexpected normalized endpoint: endpoint=%q host=%q", endpoint, host)
	}
}

func TestNormalizeEndpointRejectsUnsafeOrUnresolvableValues(t *testing.T) {
	for _, value := range []string{"", "https://registry.example.com", "registry.example.com/path", "127.0.0.1:5000", "registry"} {
		if _, _, err := NormalizeEndpoint(value); err == nil {
			t.Errorf("NormalizeEndpoint(%q) accepted an invalid endpoint", value)
		}
	}
}

func TestValidateResourceQuantities(t *testing.T) {
	if err := ValidateResourceQuantities("100m", "500m", "256Mi", "1Gi"); err != nil {
		t.Fatalf("valid resource quantities rejected: %v", err)
	}
	for _, test := range []struct {
		name string
		args [4]string
	}{
		{name: "invalid quantity", args: [4]string{"fast", "500m", "256Mi", "1Gi"}},
		{name: "request over limit", args: [4]string{"500m", "100m", "256Mi", "1Gi"}},
		{name: "memory request over limit", args: [4]string{"100m", "500m", "2Gi", "1Gi"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateResourceQuantities(test.args[0], test.args[1], test.args[2], test.args[3]); err == nil {
				t.Fatal("invalid resource quantities were accepted")
			}
		})
	}
}

func TestValidateVerificationImageRequiresConfiguredRegistry(t *testing.T) {
	if err := ValidateVerificationImage("registry.example.com", "registry.example.com/cylism-manager:1.0.0"); err != nil {
		t.Fatalf("matching verification image rejected: %v", err)
	}
	for _, image := range []string{"", "alpine:3.20", "registry.example.com"} {
		if err := ValidateVerificationImage("registry.example.com", image); err == nil {
			t.Errorf("ValidateVerificationImage accepted %q", image)
		}
	}
}

func TestCertificateDomainsCoverHostname(t *testing.T) {
	tests := []struct {
		domains []string
		host    string
		want    bool
	}{
		{[]string{"registry.example.com"}, "registry.example.com", true},
		{[]string{"*.example.com"}, "registry.example.com", true},
		{[]string{"*.example.com"}, "deep.registry.example.com", false},
		{[]string{"other.example.com"}, "registry.example.com", false},
	}
	for _, test := range tests {
		if got := CertificateDomainsCoverHostname(test.domains, test.host); got != test.want {
			t.Errorf("CertificateDomainsCoverHostname(%v, %q)=%v, want %v", test.domains, test.host, got, test.want)
		}
	}
}

func TestValidationErrorsRemainOperatorReadable(t *testing.T) {
	_, _, err := NormalizeEndpoint("https://registry.example.com")
	if err == nil || !strings.Contains(err.Error(), "制品库地址") {
		t.Fatalf("unexpected endpoint error: %v", err)
	}
}

func TestBuildManagedRegistryAppliesDefaultsAndPreservesUpdateOwnership(t *testing.T) {
	created, host, err := BuildManagedRegistry(ManagedRegistryInput{
		Name: "平台制品库", Endpoint: "registry.example.com", RegistryImage: "registry:2",
		DataNode: "node-a", PVCName: "registry-data", VerificationImage: "registry.example.com/app:1.0.0",
		CertificateName: "registry-cert", PullUsername: "pull", PullPassword: "long-enough-password",
	}, nil)
	if err != nil {
		t.Fatalf("BuildManagedRegistry returned error: %v", err)
	}
	if host != "registry.example.com" || created.Namespace != ManagedRegistryNamespace || created.ResourceName != ManagedRegistryResourceName || created.CPURequest != "100m" || created.MemoryLimit != "1Gi" {
		t.Fatalf("unexpected normalized registry: %#v, host=%q", created, host)
	}
	created.ID, created.CreatedBy, created.CreatedAt = 9, 3, time.Now()
	updated, _, err := BuildManagedRegistry(ManagedRegistryInput{
		Name: "平台制品库", Endpoint: "registry.example.com", RegistryImage: "registry:2",
		DataNode: "node-a", PVCName: "registry-data", VerificationImage: "registry.example.com/app:1.0.1",
		CertificateName: "registry-cert", PullUsername: "pull",
	}, created)
	if err != nil || updated.ID != 9 || updated.CreatedBy != 3 {
		t.Fatalf("update ownership was not preserved: registry=%#v err=%v", updated, err)
	}
}

func TestBuildManagedRegistryRejectsUnsafeUpdateAndKeepsManagedWording(t *testing.T) {
	current := &model.ManagedOCIRegistry{PVCName: "registry-data", DataNode: "node-a"}
	input := ManagedRegistryInput{Name: "平台制品库", Endpoint: "registry.example.com", RegistryImage: "registry:2", DataNode: "node-b", PVCName: "registry-data", VerificationImage: "registry.example.com/app:1", CertificateName: "registry-cert", PullUsername: "pull"}
	if _, _, err := BuildManagedRegistry(input, current); err == nil || !strings.Contains(err.Error(), "不可修改") {
		t.Fatalf("expected immutable node error, got %v", err)
	}
	input.DataNode = "node-a"
	input.VerificationImage = "docker.io/library/alpine:3.20"
	if _, _, err := BuildManagedRegistry(input, current); err == nil || !strings.Contains(err.Error(), "制品库地址") {
		t.Fatalf("expected managed verification wording, got %v", err)
	}
}

func TestManagedRegistryAssociationsFollowRegistryTransport(t *testing.T) {
	registry := &model.ManagedOCIRegistry{ID: 7, Name: "平台制品库", Endpoint: "registry.example.com", VerificationImage: "registry.example.com/app:1", PullUsername: "pull", EncryptedCredential: "encrypted", CreatedBy: 2}
	image, mirror := ManagedRegistryAssociations(registry)
	if image.ManagedRegistryID == nil || *image.ManagedRegistryID != 7 || image.AuthType != "basic" || !strings.Contains(mirror.Endpoints, "https://registry.example.com") {
		t.Fatalf("unexpected managed associations: image=%#v mirror=%#v", image, mirror)
	}
	registry.InsecureHTTP = true
	_, mirror = ManagedRegistryAssociations(registry)
	if !strings.Contains(mirror.Endpoints, "http://registry.example.com") {
		t.Fatalf("expected HTTP mirror endpoint: %#v", mirror)
	}
}

func TestManagedRegistryServicePersistsOneOwnedRegistry(t *testing.T) {
	database, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	service := NewManagedRegistryService(repository.NewManagedRegistryRepository(database), []byte("01234567890123456789012345678901"))
	registry, _, err := BuildManagedRegistry(ManagedRegistryInput{Name: "平台制品库", Endpoint: "registry.example.com", RegistryImage: "registry:2", DataNode: "node-a", PVCName: "registry-data", VerificationImage: "registry.example.com/app:1", CertificateName: "registry-cert", PullUsername: "pull", PullPassword: "long-enough-password"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PersistCreate(registry, "long-enough-password", 1, nil); err != nil {
		t.Fatalf("PersistCreate failed: %v", err)
	}
	if registry.ID == 0 || registry.EncryptedCredential == "" || registry.ImageRegistryID == nil || registry.NodeRegistryMirrorID == nil {
		t.Fatalf("managed ownership not persisted: %#v", registry)
	}
	if err := service.PersistCreate(registry, "long-enough-password", 1, nil); err == nil || !strings.Contains(err.Error(), "仅支持一个") {
		t.Fatalf("expected single registry constraint, got %v", err)
	}
}
