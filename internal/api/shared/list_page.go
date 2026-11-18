package shared

import (
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// PaginationRequested keeps pagination opt-in for existing array consumers.
func PaginationRequested(c *gin.Context) bool {
	_, limit := c.GetQuery("limit")
	_, offset := c.GetQuery("offset")
	return limit || offset
}

// InventoryNamespace retains the complete namespace facet for paginated lists.
func InventoryNamespace(c *gin.Context) string {
	if PaginationRequested(c) {
		return ""
	}
	return c.Query("namespace")
}

// ResourceListKey contains only the inventory fields needed for filtering.
type ResourceListKey struct {
	Name, Namespace, Node, Status string
	Restarts                      int32
}

// ResourceListPage filters before slicing and returns facets for the full scope.
// Kubernetes inventory is materialized by the adapter; no secret values belong here.
func ResourceListPage[T any](c *gin.Context, items []T, key func(T) ResourceListKey) {
	if !PaginationRequested(c) {
		Success(c, items)
		return
	}
	namespaces, nodes, statuses := map[string]bool{}, map[string]bool{}, map[string]bool{}
	filtered := make([]T, 0, len(items))
	query := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	for _, item := range items {
		k := key(item)
		if k.Namespace != "" {
			namespaces[k.Namespace] = true
		}
		if k.Node != "" {
			nodes[k.Node] = true
		}
		if k.Status != "" {
			statuses[k.Status] = true
		}
		if ns := c.Query("namespace"); ns != "" && ns != k.Namespace {
			continue
		}
		if node := c.Query("node"); node == "__unscheduled__" {
			if k.Node != "" {
				continue
			}
		} else if node != "" && node != k.Node {
			continue
		}
		if status := c.Query("status"); status != "" && status != k.Status {
			continue
		}
		if c.Query("restarts_only") == "true" && k.Restarts == 0 {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(k.Name), query) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := key(filtered[i]), key(filtered[j])
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
	limit, offset := LimitOffset(c, 10, 100)
	start := min(offset, len(filtered))
	keys := func(values map[string]bool) []string {
		result := make([]string, 0, len(values))
		for value := range values {
			result = append(result, value)
		}
		sort.Strings(result)
		return result
	}
	Success(c, gin.H{"items": filtered[start:min(start+limit, len(filtered))], "total": len(filtered), "limit": limit, "offset": offset, "namespaces": keys(namespaces), "nodes": keys(nodes), "statuses": keys(statuses)})
}

// ListPage preserves legacy array responses and the caller's established order.
// Callers must authorize and filter the complete scope before passing items.
func ListPage[T any](c *gin.Context, items []T) {
	if !PaginationRequested(c) {
		Success(c, items)
		return
	}
	limit, offset := LimitOffset(c, 10, 100)
	start := min(offset, len(items))
	page := append(make([]T, 0, min(limit, len(items)-start)), items[start:min(start+limit, len(items))]...)
	Success(c, gin.H{"items": page, "total": len(items), "limit": limit, "offset": offset})
}
