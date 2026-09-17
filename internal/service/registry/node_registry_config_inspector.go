package registry

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"sigs.k8s.io/yaml"
)

const (
	NodeRegistryConfigStateMatching    = "matching"
	NodeRegistryConfigStateMissing     = "missing"
	NodeRegistryConfigStateDrifted     = "drifted"
	NodeRegistryConfigStateUnsupported = "unsupported"
	NodeRegistryConfigStateUnreachable = "unreachable"
	NodeRegistryConfigStateInvalid     = "invalid"
)

// NodeRegistryConfigInspectionReader permits HTTP adapters to depend only on
// the read-only inspection contract.
type NodeRegistryConfigInspectionReader interface {
	Inspect(context.Context) (NodeRegistryConfigInspection, error)
	InspectNode(context.Context, uint) (NodeRegistryConfigInspection, error)
}

type NodeRegistryConfigInspection struct {
	InspectedAt time.Time                `json:"inspected_at"`
	Nodes       []NodeRegistryConfigNode `json:"nodes"`
}

type NodeRegistryConfigNode struct {
	ServerID uint                 `json:"server_id"`
	Name     string               `json:"name"`
	State    string               `json:"state"`
	Detail   string               `json:"detail,omitempty"`
	Expected []SafeRegistryConfig `json:"expected,omitempty"`
	Actual   []SafeRegistryConfig `json:"actual,omitempty"`
	Missing  []string             `json:"missing,omitempty"`
	Extra    []string             `json:"extra,omitempty"`
	Changed  []string             `json:"changed,omitempty"`
}

// SafeRegistryConfig intentionally contains no credential values or URL userinfo.
type SafeRegistryConfig struct {
	Registry           string   `json:"registry"`
	Endpoints          []string `json:"endpoints"`
	AuthConfigured     bool     `json:"auth_configured"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify"`
}

type NodeRegistryConfigInspector struct {
	repository repository.NodeRegistryMirrorRepository
	ssh        SSHExecutor
	now        func() time.Time
}

func NewNodeRegistryConfigInspector(repository repository.NodeRegistryMirrorRepository, ssh SSHExecutor) *NodeRegistryConfigInspector {
	return &NodeRegistryConfigInspector{repository: repository, ssh: ssh, now: time.Now}
}

func (s *NodeRegistryConfigInspector) Inspect(ctx context.Context) (NodeRegistryConfigInspection, error) {
	return s.inspect(ctx, 0)
}

// InspectNode limits inspection to one managed cluster node. The ID is only a
// selector; host, path and command remain fixed by the server.
func (s *NodeRegistryConfigInspector) InspectNode(ctx context.Context, serverID uint) (NodeRegistryConfigInspection, error) {
	if serverID == 0 {
		return NodeRegistryConfigInspection{}, fmt.Errorf("节点 ID 无效")
	}
	return s.inspect(ctx, serverID)
}

func (s *NodeRegistryConfigInspector) inspect(ctx context.Context, selectedServerID uint) (NodeRegistryConfigInspection, error) {
	if s == nil || s.repository == nil {
		return NodeRegistryConfigInspection{}, fmt.Errorf("节点镜像源检查服务不可用")
	}
	mirrors, err := s.repository.ListNodeRegistryMirrors()
	if err != nil {
		return NodeRegistryConfigInspection{}, err
	}
	servers, err := s.repository.ListServers()
	if err != nil {
		return NodeRegistryConfigInspection{}, err
	}
	expected := expectedRegistryConfigs(mirrors)
	result := NodeRegistryConfigInspection{InspectedAt: s.now().UTC(), Nodes: make([]NodeRegistryConfigNode, 0)}
	for _, server := range servers {
		if strings.TrimSpace(server.ClusterRole) != "" && (selectedServerID == 0 || selectedServerID == server.ID) {
			result.Nodes = append(result.Nodes, NodeRegistryConfigNode{ServerID: server.ID, Name: server.Name, Expected: expected})
		}
	}
	if selectedServerID != 0 && len(result.Nodes) == 0 {
		return NodeRegistryConfigInspection{}, fmt.Errorf("节点不是可检查的集群节点")
	}
	if s.ssh == nil {
		for i := range result.Nodes {
			result.Nodes[i].State, result.Nodes[i].Detail = NodeRegistryConfigStateUnreachable, "节点 SSH 检查通道不可用"
		}
		return result, nil
	}
	semaphore := make(chan struct{}, 4)
	var group sync.WaitGroup
	for i := range result.Nodes {
		group.Add(1)
		go func(node *NodeRegistryConfigNode, server model.Server) {
			defer group.Done()
			if server.SSHAuthType != "key" || strings.TrimSpace(server.SSHKey) == "" {
				node.State, node.Detail = NodeRegistryConfigStateUnsupported, "需要已配置的 SSH 密钥认证"
				return
			}
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			output, executeErr := s.ssh.Execute(ctx, 30*time.Second, &server, nodeRegistryConfigInspectCommand())
			if executeErr != nil {
				node.State, node.Detail = NodeRegistryConfigStateUnreachable, "无法通过 SSH 读取节点配置"
				return
			}
			actual, parseErr := parseSafeRegistryConfigs(output)
			if parseErr != nil {
				node.State, node.Detail = NodeRegistryConfigStateInvalid, "节点 Registry 配置格式无效"
				return
			}
			node.Actual = actual
			classifyRegistryConfig(node)
		}(&result.Nodes[i], findServer(servers, result.Nodes[i].ServerID))
	}
	group.Wait()
	return result, nil
}

func findServer(servers []model.Server, id uint) model.Server {
	for _, server := range servers {
		if server.ID == id {
			return server
		}
	}
	return model.Server{}
}

func nodeRegistryConfigInspectCommand() string {
	return `target="/etc/rancher/k3s/registries.yaml"
if sudo -n test -f "$target"; then
  sudo -n cat "$target"
fi`
}

type k3sRegistriesConfig struct {
	Mirrors map[string]k3sRegistryMirror `json:"mirrors"`
	Configs map[string]k3sRegistryConfig `json:"configs"`
}
type k3sRegistryMirror struct {
	Endpoint []string `json:"endpoint"`
}
type k3sRegistryConfig struct {
	Auth *k3sRegistryAuth `json:"auth"`
	TLS  *k3sRegistryTLS  `json:"tls"`
}
type k3sRegistryAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}
type k3sRegistryTLS struct {
	InsecureSkipVerify bool `json:"insecure_skip_verify"`
}

func parseSafeRegistryConfigs(raw []byte) ([]SafeRegistryConfig, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var config k3sRegistriesConfig
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	configs := make([]SafeRegistryConfig, 0, len(config.Mirrors))
	for registry, mirror := range config.Mirrors {
		registry = NormalizeRegistryHost(registry)
		if registry == "" {
			continue
		}
		entry := SafeRegistryConfig{Registry: registry, Endpoints: normalizedEndpoints(mirror.Endpoint)}
		if defined, ok := config.Configs[registry]; ok {
			entry.AuthConfigured = defined.Auth != nil && (strings.TrimSpace(defined.Auth.Username) != "" || strings.TrimSpace(defined.Auth.Password) != "" || strings.TrimSpace(defined.Auth.Token) != "")
			entry.InsecureSkipVerify = defined.TLS != nil && defined.TLS.InsecureSkipVerify
		}
		configs = append(configs, entry)
	}
	sortSafeRegistryConfigs(configs)
	return configs, nil
}

func expectedRegistryConfigs(mirrors []model.NodeRegistryMirror) []SafeRegistryConfig {
	configs := make([]SafeRegistryConfig, 0, len(mirrors))
	for _, mirror := range mirrors {
		if !mirror.Enabled {
			continue
		}
		configs = append(configs, SafeRegistryConfig{Registry: NormalizeRegistryHost(mirror.Registry), Endpoints: normalizedEndpoints(SafeEndpoints(mirror.Endpoints)), AuthConfigured: strings.TrimSpace(mirror.Username) != "", InsecureSkipVerify: mirror.InsecureSkipVerify})
	}
	sortSafeRegistryConfigs(configs)
	return configs
}

func normalizedEndpoints(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		if safe := SafeEndpoint(value); safe != "" {
			seen[safe] = true
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
func sortSafeRegistryConfigs(configs []SafeRegistryConfig) {
	sort.Slice(configs, func(i, j int) bool { return configs[i].Registry < configs[j].Registry })
}
func sameEndpoints(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }

func classifyRegistryConfig(node *NodeRegistryConfigNode) {
	expected, actual := map[string]SafeRegistryConfig{}, map[string]SafeRegistryConfig{}
	for _, config := range node.Expected {
		expected[config.Registry] = config
	}
	for _, config := range node.Actual {
		actual[config.Registry] = config
	}
	for registry, wanted := range expected {
		got, found := actual[registry]
		if !found {
			node.Missing = append(node.Missing, registry)
			continue
		}
		if !sameEndpoints(wanted.Endpoints, got.Endpoints) || wanted.AuthConfigured != got.AuthConfigured || wanted.InsecureSkipVerify != got.InsecureSkipVerify {
			node.Changed = append(node.Changed, registry)
		}
	}
	for registry := range actual {
		if _, found := expected[registry]; !found {
			node.Extra = append(node.Extra, registry)
		}
	}
	sort.Strings(node.Missing)
	sort.Strings(node.Extra)
	sort.Strings(node.Changed)
	if len(node.Missing) == 0 && len(node.Extra) == 0 && len(node.Changed) == 0 {
		node.State = NodeRegistryConfigStateMatching
		return
	}
	if len(node.Actual) == 0 && len(node.Expected) > 0 {
		node.State = NodeRegistryConfigStateMissing
		node.Detail = "节点未发现 Registry 配置文件或有效镜像规则"
		return
	}
	node.State, node.Detail = NodeRegistryConfigStateDrifted, "节点实际 Registry 配置与平台规则不一致"
}
