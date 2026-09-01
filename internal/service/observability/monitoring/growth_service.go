package monitoring

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

type ConsumerReader interface {
	Consumers(context.Context) (map[string][]string, error)
}

func DiskGrowth(ctx context.Context, rangeName, node string, query QueryFunc, consumers ConsumerReader) (map[string]interface{}, error) {
	spec, ok := ResolveRange(rangeName)
	if !ok {
		return nil, fmt.Errorf("时间范围仅支持 1h、6h、24h 或 7d")
	}
	items := DiskGrowthQueries(PromQLWindow(spec.Window), node)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		key  string
		data interface{}
		err  error
	}
	ch := make(chan result, len(items))
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		go func(item struct{ Key, Query string }) {
			defer wg.Done()
			data, err := query(ctx, "/api/v1/query", url.Values{"query": []string{item.Query}})
			ch <- result{item.Key, data, err}
		}(item)
	}
	go func() { wg.Wait(); close(ch) }()
	raw := map[string]interface{}{}
	warnings := map[string]string{}
	for r := range ch {
		if r.err != nil {
			warnings[r.key] = r.err.Error()
		} else {
			raw[r.key] = r.data
		}
	}
	if consumers != nil {
		if c, err := consumers.Consumers(ctx); err == nil {
			raw["consumers"] = c
		} else {
			warnings["pvcs"] = "读取 PVC 当前使用者失败: " + err.Error()
		}
	}
	data := map[string]interface{}{"range": rangeName, "node": strings.TrimSpace(node), "mounts": NormalizeMountGrowth(raw["mounts"]), "pvcs": NormalizePVCGrowth(raw["pvcs"], mapFromInterface(raw["consumers"]))}
	if len(warnings) > 0 {
		data["warnings"] = warnings
	}
	return data, nil
}

func mapFromInterface(v interface{}) map[string][]string {
	if m, ok := v.(map[string][]string); ok {
		return m
	}
	return map[string][]string{}
}
