package api

import "github.com/cylism/cylism-manager/internal/service/cluster"

// clusterNodeAdapter keeps the process-wide K8s client at the HTTP
// composition edge. Cluster Service remains independent of this API package.
func clusterNodeAdapter() cluster.NodeAdapter {
	if K8s == nil {
		return nil
	}
	return K8s
}
