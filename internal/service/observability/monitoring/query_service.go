package monitoring

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
)

const MaxPromQLLength = 2048

type StatusReader interface{ Ready(context.Context) bool }

type QueryService struct {
	query  QueryFunc
	status StatusReader
}

func NewQueryService(query QueryFunc, status StatusReader) *QueryService {
	return &QueryService{query: query, status: status}
}

func (s *QueryService) ensureReady(ctx context.Context) error {
	if s == nil || s.query == nil {
		return fmt.Errorf("VictoriaMetrics 查询不可用")
	}
	if s.status != nil && !s.status.Ready(ctx) {
		return fmt.Errorf("VictoriaMetrics 存储实例尚未就绪")
	}
	return nil
}

func (s *QueryService) Query(ctx context.Context, expression string) (interface{}, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" || len(expression) > MaxPromQLLength {
		return nil, fmt.Errorf("PromQL 查询不能为空且不能超过 2048 个字符")
	}
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	return s.query(ctx, "/api/v1/query", url.Values{"query": []string{expression}})
}

func (s *QueryService) QueryRange(ctx context.Context, expression, rangeName string, end time.Time) (interface{}, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" || len(expression) > MaxPromQLLength {
		return nil, fmt.Errorf("PromQL 查询不能为空且不能超过 2048 个字符")
	}
	spec, ok := ResolveRange(rangeName)
	if !ok {
		return nil, fmt.Errorf("时间范围仅支持 1h、6h、24h 或 7d")
	}
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	return s.query(ctx, "/api/v1/query_range", RangeValues(expression, spec, end))
}

func (s *QueryService) Targets(ctx context.Context) (interface{}, error) {
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	return s.query(ctx, "/api/v1/targets", nil)
}

func (s *QueryService) Dashboard(ctx context.Context, rangeName string, end time.Time) (map[string]interface{}, error) {
	if _, ok := ResolveRange(rangeName); !ok {
		return nil, fmt.Errorf("时间范围仅支持 1h、6h、24h 或 7d")
	}
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	return Dashboard(ctx, rangeName, end, s.query)
}

func (s *QueryService) DiskGrowth(ctx context.Context, rangeName, node string, consumers ConsumerReader) (map[string]interface{}, error) {
	if _, ok := ResolveRange(rangeName); !ok {
		return nil, fmt.Errorf("时间范围仅支持 1h、6h、24h 或 7d")
	}
	node = strings.TrimSpace(node)
	if node != "" && len(validation.IsDNS1123Subdomain(node)) > 0 {
		return nil, fmt.Errorf("节点名称无效")
	}
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	return DiskGrowth(ctx, rangeName, node, s.query, consumers)
}
