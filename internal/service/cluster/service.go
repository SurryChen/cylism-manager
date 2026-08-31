package cluster

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
)

var ErrNodeBindingCleanup = errors.New("节点已从集群移除，但解除服务器绑定失败")

// NodeAdapter is the cluster-resource boundary used by server and node
// workflows. The production adapter is the K8s client; tests provide a small
// fake without importing HTTP concerns.
type NodeAdapter interface {
	ListNodeInfos() ([]k8s.NodeInfo, error)
	GetNodeLabels(name string) (*k8s.NodeLabels, error)
	UpdateNodeLabels(name string, set map[string]string, remove []string) (*k8s.NodeLabels, error)
	DrainPlan(name string) (*k8s.DrainPlan, error)
	DrainNode(name string, options k8s.DrainOptions) (*k8s.DrainResult, error)
	ForceDrainNode(name string, options k8s.ForceDrainOptions) (*k8s.DrainResult, error)
	RejoinNode(name string) (*k8s.NodeInfo, error)
	NodeRemovalCheck(name string) (*k8s.NodeRemovalCheck, error)
	DeleteNode(name string) error
}

// ServerInspector is the side-effect boundary for remote SSH checks. The
// service owns lookup and result semantics; each entry point supplies its own
// concrete SSH or Agent implementation.
type ServerInspector interface {
	Probe(server *model.Server) (reachable bool, errorMessage string)
	Precheck(server *model.Server) []Precheck
}

type ServerImporter interface {
	Hostname(server *model.Server) (string, error)
}

type MetricsInspector interface {
	ResourceStats(server *model.Server) (map[string]interface{}, error)
}

// Precheck is the transport-independent shape returned by the server join
// prerequisite workflow. JSON tags preserve the existing REST field names.
type Precheck struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

type ProbeResult struct {
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
}

type PrecheckResult struct {
	Checks  []Precheck `json:"checks"`
	AllPass bool       `json:"all_pass"`
}

type ImportResult struct {
	ServerName string `json:"server_name"`
	Hostname   string `json:"hostname"`
	NodeName   string `json:"node_name"`
	Role       string `json:"role"`
	Version    string `json:"version"`
	InternalIP string `json:"internal_ip"`
	OS         string `json:"os"`
}

// Service owns server-to-node association maintenance and node workflows that
// also update platform-owned server records.
type Service struct {
	servers   repository.ServerRepository
	nodes     NodeAdapter
	inspector ServerInspector
	importer  ServerImporter
	metrics   MetricsInspector
}

func NewService(servers repository.ServerRepository, nodes NodeAdapter) *Service {
	return &Service{servers: servers, nodes: nodes}
}

func (s *Service) WithServerInspector(inspector ServerInspector) *Service {
	s.inspector = inspector
	return s
}

func (s *Service) WithServerImporter(importer ServerImporter) *Service {
	s.importer = importer
	return s
}

func (s *Service) WithMetricsInspector(metrics MetricsInspector) *Service {
	s.metrics = metrics
	return s
}

// CreateServer persists an already validated server record. Secret encryption
// remains an explicit caller concern so this service can be reused by HTTP,
// CLI, and Agent entry points without accepting plaintext credentials.
func (s *Service) CreateServer(server *model.Server) error {
	return s.servers.CreateServer(server)
}

func (s *Service) GetServer(id uint) (*model.Server, error) {
	return s.servers.GetServer(id)
}

func (s *Service) UpdateServer(server *model.Server) error {
	return s.servers.UpdateServer(server)
}

func (s *Service) DeleteServer(id uint) error {
	return s.servers.DeleteServer(id)
}

// UnbindServer removes the platform-only server-to-node relation while
// deliberately leaving Kubernetes resources and server connection data intact.
func (s *Service) UnbindServer(id uint) (*model.Server, error) {
	server, err := s.servers.GetServer(id)
	if err != nil {
		return nil, err
	}
	server.ClusterRole = ""
	server.K8sNodeName = ""
	if err := s.servers.UpdateServer(server); err != nil {
		return nil, err
	}
	return server, nil
}

func (s *Service) ProbeServer(id uint) (*ProbeResult, error) {
	server, err := s.servers.GetServer(id)
	if err != nil {
		return nil, err
	}
	if s.inspector == nil {
		return nil, fmt.Errorf("服务器检查器未配置")
	}
	startedAt := time.Now()
	reachable, errorMessage := s.inspector.Probe(server)
	return &ProbeResult{
		Reachable: reachable,
		Error:     errorMessage,
		LatencyMS: time.Since(startedAt).Milliseconds(),
	}, nil
}

func (s *Service) PrecheckServer(id uint) (*PrecheckResult, error) {
	server, err := s.servers.GetServer(id)
	if err != nil {
		return nil, err
	}
	if s.inspector == nil {
		return nil, fmt.Errorf("服务器检查器未配置")
	}
	checks := s.inspector.Precheck(server)
	result := &PrecheckResult{Checks: checks, AllPass: true}
	for _, check := range checks {
		if !check.Pass {
			result.AllPass = false
			break
		}
	}
	return result, nil
}

func (s *Service) PreImportServer(id uint) (*ImportResult, error) {
	server, err := s.servers.GetServer(id)
	if err != nil {
		return nil, err
	}
	if s.importer == nil {
		return nil, fmt.Errorf("服务器导入检查器未配置")
	}
	if s.nodes == nil {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	hostname, err := s.importer.Hostname(server)
	if err != nil {
		return nil, fmt.Errorf("获取主机名失败: %w", err)
	}
	nodes, err := s.nodes.ListNodeInfos()
	if err != nil {
		return nil, fmt.Errorf("获取集群节点列表失败: %w", err)
	}
	var matched *k8s.NodeInfo
	for index := range nodes {
		if nodes[index].InternalIP == server.Host || strings.EqualFold(nodes[index].Name, hostname) {
			matched = &nodes[index]
			break
		}
	}
	if matched == nil {
		return nil, fmt.Errorf("集群中未找到匹配节点 (IP=%s, hostname=%s)", server.Host, hostname)
	}
	if !matched.Ready {
		return nil, fmt.Errorf("节点 %s 状态异常 (NotReady)", matched.Name)
	}
	return &ImportResult{ServerName: server.Name, Hostname: hostname, NodeName: matched.Name, Role: matched.Roles, Version: matched.Version, InternalIP: matched.InternalIP, OS: matched.OS}, nil
}

func (s *Service) ConfirmImport(id uint, hostname, role string) (*ImportResult, error) {
	server, err := s.servers.GetServer(id)
	if err != nil {
		return nil, err
	}
	server.K8sNodeName = hostname
	server.ClusterRole = role
	if err := s.servers.UpdateServer(server); err != nil {
		return nil, err
	}
	return &ImportResult{ServerName: server.Name, Hostname: hostname, NodeName: hostname, Role: role}, nil
}

func (s *Service) ServerStats(id uint) (map[string]interface{}, error) {
	server, err := s.servers.GetServer(id)
	if err != nil {
		return nil, err
	}
	if s.metrics == nil {
		return nil, fmt.Errorf("服务器指标检查器未配置")
	}
	return s.metrics.ResourceStats(server)
}

func (s *Service) ResourceStats() ([]map[string]interface{}, error) {
	servers, err := s.servers.ListServers()
	if err != nil {
		return nil, err
	}
	if s.metrics == nil {
		return nil, fmt.Errorf("服务器指标检查器未配置")
	}
	results := make([]map[string]interface{}, len(servers))
	semaphore := make(chan struct{}, 3)
	var group sync.WaitGroup
	for index := range servers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			started := time.Now()
			stats, statsErr := s.metrics.ResourceStats(&servers[index])
			if stats == nil {
				stats = map[string]interface{}{}
			}
			stats["server_id"] = servers[index].ID
			stats["server_name"] = servers[index].Name
			stats["sampled_at"] = started.UTC().Format(time.RFC3339)
			stats["duration_ms"] = time.Since(started).Milliseconds()
			if statsErr != nil {
				stats["status"] = "unreachable"
				stats["error"] = statsErr.Error()
			} else if stats["status"] == nil {
				stats["status"] = "ready"
			}
			results[index] = stats
		}(index)
	}
	group.Wait()
	return results, nil
}

// ListServers returns registered servers and reconciles stale Kubernetes node
// associations when the cluster is reachable. A transient Kubernetes API
// error deliberately preserves existing bindings.
func (s *Service) ListServers() ([]model.Server, error) {
	servers, err := s.servers.ListServers()
	if err != nil || s.nodes == nil {
		return servers, err
	}
	nodes, err := s.nodes.ListNodeInfos()
	if err != nil {
		return servers, nil
	}
	existing := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		existing[node.Name] = struct{}{}
	}
	for index := range servers {
		server := &servers[index]
		if server.ClusterRole == "" && server.K8sNodeName == "" {
			continue
		}
		if server.K8sNodeName != "" {
			if _, found := existing[server.K8sNodeName]; found {
				continue
			}
		}
		if server.K8sNodeName == "" {
			server.ClusterRole = ""
			if err := s.servers.UpdateServer(server); err != nil {
				continue
			}
			continue
		}
		if err := s.servers.UnbindServersFromClusterNode(server.K8sNodeName); err != nil {
			continue
		}
		server.ClusterRole = ""
		server.K8sNodeName = ""
	}
	return servers, nil
}

func (s *Service) UpdateNodeLabels(name string, set map[string]string, remove []string) (*k8s.NodeLabels, error) {
	if s.nodes == nil {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	return s.nodes.UpdateNodeLabels(name, set, remove)
}

func (s *Service) ListNodes() ([]k8s.NodeInfo, error) {
	if s.nodes == nil {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	return s.nodes.ListNodeInfos()
}

func (s *Service) GetNodeLabels(name string) (*k8s.NodeLabels, error) {
	if s.nodes == nil {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	return s.nodes.GetNodeLabels(name)
}

func (s *Service) GetDrainPlan(name string) (*k8s.DrainPlan, error) {
	if s.nodes == nil {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	return s.nodes.DrainPlan(name)
}

func (s *Service) DrainNode(name string, options k8s.DrainOptions) (*k8s.DrainResult, error) {
	if s.nodes == nil {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	return s.nodes.DrainNode(name, options)
}

// ForceDrainNode protects this destructive workflow with the same explicit
// acknowledgement and exact target confirmation required by the HTTP API.
func (s *Service) ForceDrainNode(name string, options k8s.ForceDrainOptions) (*k8s.DrainResult, error) {
	if s.nodes == nil {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	if !options.AcknowledgeRisk || strings.TrimSpace(options.ConfirmNodeName) != name {
		return nil, fmt.Errorf("请确认风险并输入目标节点名")
	}
	options.ConfirmNodeName = strings.TrimSpace(options.ConfirmNodeName)
	return s.nodes.ForceDrainNode(name, options)
}

func (s *Service) RejoinNode(name string) (*k8s.NodeInfo, error) {
	if s.nodes == nil {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	return s.nodes.RejoinNode(name)
}

func (s *Service) GetRemovalCheck(name string) (*k8s.NodeRemovalCheck, error) {
	if s.nodes == nil {
		return nil, fmt.Errorf("Kubernetes 集群未连接")
	}
	return s.nodes.NodeRemovalCheck(name)
}

// RemoveNode deletes the Kubernetes node only after the adapter's own
// preconditions pass, then removes the stale platform association.
func (s *Service) RemoveNode(name string) error {
	if s.nodes == nil {
		return fmt.Errorf("Kubernetes 集群未连接")
	}
	if err := s.nodes.DeleteNode(name); err != nil {
		return err
	}
	if err := s.servers.UnbindServersFromClusterNode(name); err != nil {
		return fmt.Errorf("%w: %v", ErrNodeBindingCleanup, err)
	}
	return nil
}
