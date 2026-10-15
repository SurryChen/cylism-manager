package registry

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestResolveRegistryDoesNotSelectProxyFromAnotherCluster(t *testing.T) {
	proxies := []model.RegistryProxy{{ClusterID: 7, Registry: "docker.io", EndpointHost: "100.76.53.48", NodePort: 30500}}
	if _, found := ResolveRegistry("docker.io", nil, proxies); found {
		t.Fatal("cross-cluster proxy was selected for the default cluster")
	}
}
