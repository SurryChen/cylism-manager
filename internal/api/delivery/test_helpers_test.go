package delivery

import (
	"encoding/json"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/repository"
	platformservice "github.com/cylism/cylism-manager/internal/service/platform"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
)

func responseID(t *testing.T, body []byte) uint {
	t.Helper()
	var v struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	return v.Data.ID
}

func newTestPlatformHandler(store repository.PlatformEndpointRepository, encKey []byte, client platformKubernetes) *PlatformHandler {
	return NewPlatformHandlerWithDependencies(store, client, platformservice.NewReleaseService(store, encKey, client))
}

func newTestNodeRegistryMirrorHandler(repo repository.NodeRegistryMirrorRepository, encKey []byte, apply registryservice.NodeMirrorApplier) *NodeRegistryMirrorHandler {
	return NewNodeRegistryMirrorHandlerWithDependencies(encKey, apply, registryservice.NewMirrorService(repo, encKey))
}

func newTestManagedOCIRegistryHandler(repo repository.ManagedRegistryRepository, encKey []byte, resources k8sclient.ManagedRegistryResourceReconciler, status k8sclient.ManagedRegistryStatusReader, apply registryservice.NodeMirrorApplier) *ManagedOCIRegistryHandler {
	return NewManagedOCIRegistryHandlerWithDependencies(repo, resources, status, apply, registryservice.NewManagedRegistryService(repo, encKey))
}

func newTestRegistryProxyHandler(repo repository.RegistryProxyRepository, encKey []byte, resources k8sclient.RegistryProxyResourceReconciler, diagnostics k8sclient.RegistryProxyDiagnostics) *RegistryProxyHandler {
	return NewRegistryProxyHandlerWithDependencies(repo, encKey, resources, diagnostics, registryservice.NewProxyService(repo, encKey))
}
