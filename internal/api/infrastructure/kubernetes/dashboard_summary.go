package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DashboardSummaryService reads the small cluster summary used by the home
// page. It keeps the expensive cluster-wide lists out of the HTTP handler and
// coalesces bursts of dashboard requests into one Kubernetes refresh.
type DashboardSummaryService struct {
	adapter K8sResourceAdapter
	ttl     time.Duration
	mu      sync.Mutex
	value   map[string]interface{}
	updated time.Time
	refresh chan struct{}
}

func NewDashboardSummaryService(adapter K8sResourceAdapter, ttl time.Duration) *DashboardSummaryService {
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	return &DashboardSummaryService{adapter: adapter, ttl: ttl}
}

func (s *DashboardSummaryService) Get(ctx context.Context) (map[string]interface{}, error) {
	if s == nil || s.adapter == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	for {
		s.mu.Lock()
		if s.value != nil && time.Since(s.updated) < s.ttl {
			value := cloneSummary(s.value)
			s.mu.Unlock()
			return value, nil
		}
		if s.refresh != nil {
			wait := s.refresh
			s.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		s.refresh = make(chan struct{})
		wait := s.refresh
		s.mu.Unlock()

		value, err := s.refreshValue(ctx)
		s.mu.Lock()
		if err == nil {
			s.value, s.updated = value, time.Now()
		}
		close(wait)
		s.refresh = nil
		s.mu.Unlock()
		return value, err
	}
}

func (s *DashboardSummaryService) refreshValue(ctx context.Context) (map[string]interface{}, error) {
	type result struct {
		key   string
		value interface{}
		err   error
	}
	results := make(chan result, 5)
	go func() { nodes, err := s.adapter.ListNodeInfosContext(ctx); results <- result{"nodes", nodes, err} }()
	go func() {
		list, err := s.adapter.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			results <- result{"namespaces", nil, err}
			return
		}
		results <- result{"namespaces", len(list.Items), nil}
	}()
	go func() {
		list, err := s.adapter.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err != nil {
			results <- result{"pods", nil, err}
			return
		}
		ready := 0
		for i := range list.Items {
			if podReady(&list.Items[i]) {
				ready++
			}
		}
		results <- result{"pods", []int{len(list.Items), ready}, nil}
	}()
	go func() {
		list, err := s.adapter.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
		if err != nil {
			results <- result{"deployments", nil, err}
			return
		}
		ready := 0
		for i := range list.Items {
			expected := int32(0)
			if list.Items[i].Spec.Replicas != nil {
				expected = *list.Items[i].Spec.Replicas
			}
			if list.Items[i].Status.ReadyReplicas == expected {
				ready++
			}
		}
		results <- result{"deployments", []int{len(list.Items), ready}, nil}
	}()
	go func() {
		list, err := s.adapter.CoreV1().Services("").List(ctx, metav1.ListOptions{})
		if err != nil {
			results <- result{"services", nil, err}
			return
		}
		results <- result{"services", len(list.Items), nil}
	}()

	out := map[string]interface{}{"partial": false, "errors": map[string]string{}}
	errs := out["errors"].(map[string]string)
	for range 5 {
		r := <-results
		if r.err != nil {
			errs[r.key] = r.err.Error()
			out["partial"] = true
			continue
		}
		switch r.key {
		case "nodes":
			nodes := r.value.([]k8sclient.NodeInfo)
			out["nodes_total"] = len(nodes)
			readyNodes := 0
			controlPlaneNodes := 0
			workerNodes := 0
			var cpuCores int64
			var memoryMB int64
			versions := map[string]struct{}{}
			for _, node := range nodes {
				if node.Ready {
					readyNodes++
				}
				if node.Roles == "control-plane" {
					controlPlaneNodes++
				} else {
					workerNodes++
				}
				cpuCores += node.CPUCores
				memoryMB += node.MemoryMB
				if node.Version != "" {
					versions[normalizeVersion(node.Version)] = struct{}{}
				}
			}
			out["nodes_ready"] = readyNodes
			out["nodes_not_ready"] = len(nodes) - readyNodes
			out["control_plane_nodes"] = controlPlaneNodes
			out["worker_nodes"] = workerNodes
			out["cpu_cores_total"] = cpuCores
			out["memory_mb_total"] = memoryMB
			out["version_consistent"] = len(versions) <= 1
			if len(versions) == 1 {
				for version := range versions {
					out["version"] = version
				}
			} else if len(versions) > 1 {
				out["version"] = "多版本"
			}
		case "namespaces":
			out["namespaces"] = r.value
		case "pods":
			values := r.value.([]int)
			out["pods_total"], out["pods_ready"] = values[0], values[1]
		case "deployments":
			values := r.value.([]int)
			out["deployments_total"], out["deployments_ready"] = values[0], values[1]
		case "services":
			out["services_total"] = r.value
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(errs) == 0 {
		delete(out, "errors")
	}
	return out, nil
}

func podReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
func normalizeVersion(version string) string { return strings.TrimSpace(version) }
func cloneSummary(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		if m, ok := v.(map[string]string); ok {
			cp := map[string]string{}
			for mk, mv := range m {
				cp[mk] = mv
			}
			out[k] = cp
		} else {
			out[k] = v
		}
	}
	return out
}
