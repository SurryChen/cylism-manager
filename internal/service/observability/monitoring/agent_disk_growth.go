package monitoring

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// AgentDiskGrowthService applies the narrower contract exposed to Runtime
// agents: only bounded ranges and a validated node name are accepted.
type AgentDiskGrowthService struct {
	Query     *QueryService
	Consumers ConsumerReader
}

func NewAgentDiskGrowthService(query *QueryService, consumers ConsumerReader) *AgentDiskGrowthService {
	return &AgentDiskGrowthService{Query: query, Consumers: consumers}
}

func (s *AgentDiskGrowthService) QueryNode(ctx context.Context, rangeName, node string) (map[string]interface{}, error) {
	if s == nil || s.Query == nil {
		return nil, fmt.Errorf("monitoring data store unavailable")
	}
	node = strings.TrimSpace(node)
	if node == "" || len(validation.IsDNS1123Subdomain(node)) > 0 {
		return nil, fmt.Errorf("invalid node")
	}
	if rangeName != "6h" && rangeName != "24h" {
		return nil, fmt.Errorf("range must be 6h or 24h")
	}
	return s.Query.DiskGrowth(ctx, rangeName, node, s.Consumers)
}
