package bootstrap

import (
	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	monitoringservice "github.com/cylism/cylism-manager/internal/service/observability/monitoring"
	storageservice "github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/cylism/cylism-manager/internal/store"
)

// Services contains all application services assembled by the composition
// root. Handlers consume these instances instead of constructing services.
type Services struct {
	Cluster    *cluster.Service
	Network    *networkservice.Service
	Storage    *storageservice.Service
	Monitoring *monitoringservice.QueryService
}

func BuildServices(db *store.Store, adapters KubernetesAdapters, encKey []byte) Services {
	clusterSvc := cluster.NewService(db, adapters.Nodes).
		WithServerInspector(infrastructureapi.ServerInspector{EncKey: encKey}).
		WithServerImporter(infrastructureapi.ServerInspector{EncKey: encKey}).
		WithMetricsInspector(infrastructureapi.ServerMetricsInspector{EncKey: encKey})
	networkSvc := networkservice.NewService(db, db).
		WithIngressAdapter(adapters.Network.Ingress).
		WithStandardIngressAdapter(adapters.Network.StandardIngress).
		WithDNSAdapter(adapters.Network.DNS).
		WithCertificateAdapter(adapters.Network.Certificate)
	return Services{
		Cluster:    clusterSvc,
		Network:    networkSvc,
		Storage:    storageservice.NewService(adapters.Storage, db, db),
		Monitoring: monitoringservice.NewQueryService(adapters.Monitoring.Query, adapters.Monitoring.Status),
	}
}
