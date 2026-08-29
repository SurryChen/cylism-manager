package registry

import (
	"strings"
	"testing"
)

func TestNormalizeImageRegistryEndpoint(t *testing.T) {
	endpoint, err := NormalizeImageRegistryEndpoint(" https://Harbor.Example.com:5443/ ")
	if err != nil {
		t.Fatalf("NormalizeImageRegistryEndpoint returned error: %v", err)
	}
	if endpoint != "Harbor.Example.com:5443" {
		t.Fatalf("unexpected endpoint: %q", endpoint)
	}
	if _, err := NormalizeImageRegistryEndpoint("harbor.example.com/library"); err == nil {
		t.Fatal("endpoint with a path was accepted")
	}
}

func TestVerificationImageReferenceRequiresMatchingRegistry(t *testing.T) {
	ref, err := VerificationImageReference("docker.io", "docker.io/library/busybox:1.36", "验证镜像必须属于当前镜像仓库地址")
	if err != nil {
		t.Fatalf("VerificationImageReference returned error: %v", err)
	}
	if ref.Name() != "index.docker.io/library/busybox:1.36" {
		t.Fatalf("unexpected reference: %q", ref.Name())
	}
	if _, err := VerificationImageReference("docker.io", "ghcr.io/example/busybox:1.36", "验证镜像必须属于当前镜像仓库地址"); err == nil || !strings.Contains(err.Error(), "当前镜像仓库地址") {
		t.Fatalf("expected registry mismatch error, got %v", err)
	}
}

func TestSameRegistryNormalizesDockerAliases(t *testing.T) {
	for _, pair := range [][2]string{{"docker.io", "index.docker.io"}, {"docker.io", "registry-1.docker.io"}, {"registry.example.com", "REGISTRY.EXAMPLE.COM"}} {
		if !SameRegistry(pair[0], pair[1]) {
			t.Errorf("SameRegistry(%q, %q) = false", pair[0], pair[1])
		}
	}
	if SameRegistry("docker.io", "ghcr.io") {
		t.Fatal("different registries matched")
	}
}

func TestMirrorVerificationReferencePreservesImagePathAndTag(t *testing.T) {
	ref, err := MirrorVerificationReference("docker.io", "docker.io/rancher/mirrored-library-traefik:3.7.4", "http://mirror.internal:5000")
	if err != nil {
		t.Fatalf("MirrorVerificationReference returned error: %v", err)
	}
	if ref.Name() != "mirror.internal:5000/rancher/mirrored-library-traefik:3.7.4" {
		t.Fatalf("unexpected mirror reference: %q", ref.Name())
	}
}

func TestValidateMirrorEndpointURL(t *testing.T) {
	if err := ValidateMirrorEndpointURL("https://mirror.internal:5000"); err != nil {
		t.Fatalf("valid mirror endpoint rejected: %v", err)
	}
	if err := ValidateMirrorEndpointURL("mirror.internal:5000"); err == nil || !strings.Contains(err.Error(), "HTTP 或 HTTPS") {
		t.Fatalf("invalid mirror endpoint accepted: %v", err)
	}
}
