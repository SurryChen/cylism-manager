package api

import (
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/service/cluster"
)

// clusterNodeAdapter keeps the process-wide K8s client at the HTTP
// composition edge. Cluster Service remains independent of this API package.
func clusterNodeAdapter(client *k8s.Client) cluster.NodeAdapter {
	if client == nil {
		return nil
	}
	return client
}
