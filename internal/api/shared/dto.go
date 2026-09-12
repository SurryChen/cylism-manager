package shared

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// ServerView is the public representation of a managed server. Credentials,
// encrypted material, and credential hashes are intentionally excluded.
type ServerView struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	SSHPort     int    `json:"ssh_port"`
	SSHUser     string `json:"ssh_user"`
	SSHAuthType string `json:"ssh_auth_type"`
	ClusterRole string `json:"cluster_role"`
	K8sNodeName string `json:"k8s_node_name"`
}

func ServerDTO(server *model.Server) *ServerView {
	if server == nil {
		return nil
	}
	return &ServerView{ID: server.ID, Name: server.Name, Host: server.Host, SSHPort: server.SSHPort, SSHUser: server.SSHUser, SSHAuthType: server.SSHAuthType, ClusterRole: server.ClusterRole, K8sNodeName: server.K8sNodeName}
}

func ServersDTO(servers []model.Server) []ServerView {
	views := make([]ServerView, 0, len(servers))
	for i := range servers {
		if view := ServerDTO(&servers[i]); view != nil {
			views = append(views, *view)
		}
	}
	return views
}

// ImageRegistryView is the public representation of an image registry. The
// encrypted credential is never exposed; CredentialConfigured is a safe hint.
type ImageRegistryView struct {
	ID                   uint       `json:"id"`
	Name                 string     `json:"name"`
	Endpoint             string     `json:"endpoint"`
	VerificationImage    string     `json:"verification_image"`
	AuthType             string     `json:"auth_type"`
	Username             string     `json:"username"`
	Enabled              bool       `json:"enabled"`
	LastVerifiedAt       *time.Time `json:"last_verified_at,omitempty"`
	LastVerifyStatus     string     `json:"last_verify_status"`
	LastVerifyError      string     `json:"last_verify_error,omitempty"`
	CreatedBy            uint       `json:"created_by"`
	ManagedRegistryID    *uint      `json:"managed_registry_id,omitempty"`
	CredentialConfigured bool       `json:"credential_configured"`
}

func ImageRegistryDTO(registry *model.ImageRegistry) *ImageRegistryView {
	if registry == nil {
		return nil
	}
	return &ImageRegistryView{ID: registry.ID, Name: registry.Name, Endpoint: registry.Endpoint, VerificationImage: registry.VerificationImage, AuthType: registry.AuthType, Username: registry.Username, Enabled: registry.Enabled, LastVerifiedAt: registry.LastVerifiedAt, LastVerifyStatus: registry.LastVerifyStatus, LastVerifyError: registry.LastVerifyError, CreatedBy: registry.CreatedBy, ManagedRegistryID: registry.ManagedRegistryID, CredentialConfigured: registry.CredentialConfigured}
}

func ImageRegistriesDTO(registries []model.ImageRegistry) []ImageRegistryView {
	views := make([]ImageRegistryView, 0, len(registries))
	for i := range registries {
		if view := ImageRegistryDTO(&registries[i]); view != nil {
			views = append(views, *view)
		}
	}
	return views
}

type NodeRegistryMirrorView struct {
	ID                   uint                         `json:"id"`
	Name                 string                       `json:"name"`
	Registry             string                       `json:"registry"`
	Endpoints            string                       `json:"endpoints"`
	VerificationImage    string                       `json:"verification_image"`
	Username             string                       `json:"username"`
	InsecureSkipVerify   bool                         `json:"insecure_skip_verify"`
	Enabled              bool                         `json:"enabled"`
	CreatedBy            uint                         `json:"created_by"`
	ManagedRegistryID    *uint                        `json:"managed_registry_id,omitempty"`
	LastVerifiedAt       *time.Time                   `json:"last_verified_at,omitempty"`
	LastVerifyStatus     string                       `json:"last_verify_status"`
	LastVerifyError      string                       `json:"last_verify_error,omitempty"`
	LastAppliedAt        *time.Time                   `json:"last_applied_at,omitempty"`
	LastApplyStatus      string                       `json:"last_apply_status"`
	LastApplyError       string                       `json:"last_apply_error,omitempty"`
	CredentialConfigured bool                         `json:"credential_configured"`
	NodeStatuses         []NodeRegistryMirrorNodeView `json:"node_statuses,omitempty"`
}

type NodeRegistryMirrorNodeView struct {
	ID        uint        `json:"id"`
	MirrorID  uint        `json:"mirror_id"`
	ServerID  uint        `json:"server_id"`
	Status    string      `json:"status"`
	Detail    string      `json:"detail,omitempty"`
	AppliedAt *time.Time  `json:"applied_at,omitempty"`
	Server    *ServerView `json:"server,omitempty"`
}

func nodeRegistryMirrorNodeDTO(node model.NodeRegistryMirrorNode) NodeRegistryMirrorNodeView {
	view := NodeRegistryMirrorNodeView{ID: node.ID, MirrorID: node.MirrorID, ServerID: node.ServerID, Status: node.Status, Detail: node.Detail, AppliedAt: node.AppliedAt}
	if node.Server.ID != 0 {
		view.Server = ServerDTO(&node.Server)
	}
	return view
}

func NodeRegistryMirrorDTO(mirror *model.NodeRegistryMirror) *NodeRegistryMirrorView {
	if mirror == nil {
		return nil
	}
	nodes := make([]NodeRegistryMirrorNodeView, 0, len(mirror.NodeStatuses))
	for _, node := range mirror.NodeStatuses {
		nodes = append(nodes, nodeRegistryMirrorNodeDTO(node))
	}
	return &NodeRegistryMirrorView{ID: mirror.ID, Name: mirror.Name, Registry: mirror.Registry, Endpoints: mirror.Endpoints, VerificationImage: mirror.VerificationImage, Username: mirror.Username, InsecureSkipVerify: mirror.InsecureSkipVerify, Enabled: mirror.Enabled, CreatedBy: mirror.CreatedBy, ManagedRegistryID: mirror.ManagedRegistryID, LastVerifiedAt: mirror.LastVerifiedAt, LastVerifyStatus: mirror.LastVerifyStatus, LastVerifyError: mirror.LastVerifyError, LastAppliedAt: mirror.LastAppliedAt, LastApplyStatus: mirror.LastApplyStatus, LastApplyError: mirror.LastApplyError, CredentialConfigured: mirror.CredentialConfigured, NodeStatuses: nodes}
}

func NodeRegistryMirrorsDTO(items []model.NodeRegistryMirror) []NodeRegistryMirrorView {
	views := make([]NodeRegistryMirrorView, 0, len(items))
	for i := range items {
		if v := NodeRegistryMirrorDTO(&items[i]); v != nil {
			views = append(views, *v)
		}
	}
	return views
}

type ManagedOCIRegistryView struct {
	ID                   uint       `json:"id"`
	Name                 string     `json:"name"`
	Namespace            string     `json:"namespace"`
	ResourceName         string     `json:"resource_name"`
	Endpoint             string     `json:"endpoint"`
	VerificationImage    string     `json:"verification_image"`
	RegistryImage        string     `json:"registry_image"`
	DataNode             string     `json:"data_node"`
	StorageClassName     string     `json:"storage_class_name"`
	PVCName              string     `json:"pvc_name"`
	StorageSize          string     `json:"storage_size"`
	CPURequest           string     `json:"cpu_request"`
	CPULimit             string     `json:"cpu_limit"`
	MemoryRequest        string     `json:"memory_request"`
	MemoryLimit          string     `json:"memory_limit"`
	InsecureHTTP         bool       `json:"insecure_http"`
	CertificateName      string     `json:"certificate_name,omitempty"`
	TLSSecretName        string     `json:"tls_secret_name,omitempty"`
	PullUsername         string     `json:"pull_username"`
	ImageRegistryID      *uint      `json:"image_registry_id,omitempty"`
	NodeRegistryMirrorID *uint      `json:"node_registry_mirror_id,omitempty"`
	Status               string     `json:"status"`
	LastError            string     `json:"last_error,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	CreatedBy            uint       `json:"created_by"`
	CredentialConfigured bool       `json:"credential_configured"`
	PVCPhase             string     `json:"pvc_phase,omitempty"`
}

func ManagedOCIRegistryDTO(registry *model.ManagedOCIRegistry) *ManagedOCIRegistryView {
	if registry == nil {
		return nil
	}
	return &ManagedOCIRegistryView{ID: registry.ID, Name: registry.Name, Namespace: registry.Namespace, ResourceName: registry.ResourceName, Endpoint: registry.Endpoint, VerificationImage: registry.VerificationImage, RegistryImage: registry.RegistryImage, DataNode: registry.DataNode, StorageClassName: registry.StorageClassName, PVCName: registry.PVCName, StorageSize: registry.StorageSize, CPURequest: registry.CPURequest, CPULimit: registry.CPULimit, MemoryRequest: registry.MemoryRequest, MemoryLimit: registry.MemoryLimit, InsecureHTTP: registry.InsecureHTTP, CertificateName: registry.CertificateName, TLSSecretName: registry.TLSSecretName, PullUsername: registry.PullUsername, ImageRegistryID: registry.ImageRegistryID, NodeRegistryMirrorID: registry.NodeRegistryMirrorID, Status: registry.Status, LastError: registry.LastError, LastCheckedAt: registry.LastCheckedAt, CreatedBy: registry.CreatedBy, CredentialConfigured: registry.CredentialConfigured, PVCPhase: registry.PVCPhase}
}

// RuntimeView excludes both encrypted API keys while retaining the runtime
// configuration and health fields needed by the console.
type RuntimeView struct {
	ID                 uint       `json:"id"`
	Name               string     `json:"name"`
	RuntimeType        string     `json:"runtime_type"`
	DeploymentMode     string     `json:"deployment_mode"`
	RuntimeVersion     string     `json:"runtime_version,omitempty"`
	Image              string     `json:"image"`
	Namespace          string     `json:"namespace"`
	Port               int32      `json:"port"`
	HealthPath         string     `json:"health_path"`
	PVCName            string     `json:"pvc_name"`
	Storage            string     `json:"storage"`
	StorageClassName   string     `json:"storage_class_name,omitempty"`
	NodeName           string     `json:"node_name,omitempty"`
	EndpointURL        string     `json:"endpoint_url,omitempty"`
	ModelName          string     `json:"model_name,omitempty"`
	ModelBaseURL       string     `json:"model_base_url,omitempty"`
	APIStyle           string     `json:"api_style"`
	APIKeyConfigured   bool       `json:"api_key_configured"`
	Config             string     `json:"config,omitempty"`
	SecretName         string     `json:"secret_name,omitempty"`
	Status             string     `json:"status"`
	DesiredGeneration  uint       `json:"desired_generation"`
	ObservedGeneration uint       `json:"observed_generation"`
	HealthStatus       string     `json:"health_status,omitempty"`
	HealthDetail       string     `json:"health_detail,omitempty"`
	LastHealthAt       *time.Time `json:"last_health_at,omitempty"`
	AgentToolEnabled   bool       `json:"agent_tool_enabled"`
	CreatedBy          uint       `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func RuntimeDTO(runtime *model.RuntimeInstance) *RuntimeView {
	if runtime == nil {
		return nil
	}
	return &RuntimeView{ID: runtime.ID, Name: runtime.Name, RuntimeType: runtime.RuntimeType, DeploymentMode: runtime.DeploymentMode, RuntimeVersion: runtime.RuntimeVersion, Image: runtime.Image, Namespace: runtime.Namespace, Port: runtime.Port, HealthPath: runtime.HealthPath, PVCName: runtime.PVCName, Storage: runtime.Storage, StorageClassName: runtime.StorageClassName, NodeName: runtime.NodeName, EndpointURL: runtime.EndpointURL, ModelName: runtime.ModelName, ModelBaseURL: runtime.ModelBaseURL, APIStyle: runtime.APIStyle, APIKeyConfigured: runtime.APIKeyConfigured, Config: runtime.Config, SecretName: runtime.SecretName, Status: runtime.Status, DesiredGeneration: runtime.DesiredGeneration, ObservedGeneration: runtime.ObservedGeneration, HealthStatus: runtime.HealthStatus, HealthDetail: runtime.HealthDetail, LastHealthAt: runtime.LastHealthAt, AgentToolEnabled: runtime.AgentToolEnabled, CreatedBy: runtime.CreatedBy, CreatedAt: runtime.CreatedAt, UpdatedAt: runtime.UpdatedAt}
}

func RuntimesDTO(items []model.RuntimeInstance) []RuntimeView {
	views := make([]RuntimeView, 0, len(items))
	for i := range items {
		if v := RuntimeDTO(&items[i]); v != nil {
			views = append(views, *v)
		}
	}
	return views
}

type ProjectView struct {
	ID                     uint               `json:"id"`
	Name                   string             `json:"name"`
	Description            string             `json:"description"`
	DefaultImageRegistryID *uint              `json:"default_image_registry_id,omitempty"`
	OwnerID                uint               `json:"owner_id"`
	CreatedAt              time.Time          `json:"created_at"`
	UpdatedAt              time.Time          `json:"updated_at"`
	Environments           []EnvironmentView  `json:"environments,omitempty"`
	DefaultImageRegistry   *ImageRegistryView `json:"default_image_registry,omitempty"`
}
type EnvironmentView struct {
	ID                uint      `json:"id"`
	ProjectID         uint      `json:"project_id"`
	Name              string    `json:"name"`
	Namespace         string    `json:"namespace"`
	NamespaceStatus   string    `json:"namespace_status,omitempty"`
	NamespaceConflict bool      `json:"namespace_conflict,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
type ApplicationEndpointView struct {
	ID             uint      `json:"id"`
	ApplicationID  uint      `json:"application_id"`
	DomainID       uint      `json:"domain_id,omitempty"`
	Exposure       string    `json:"exposure"`
	Domain         string    `json:"domain"`
	Path           string    `json:"path"`
	ServicePort    int32     `json:"service_port"`
	Protocol       string    `json:"protocol"`
	TLSEnabled     bool      `json:"tls_enabled"`
	IngressEnabled bool      `json:"ingress_enabled"`
	TLSSecretName  string    `json:"tls_secret_name"`
	IssuerRef      string    `json:"issuer_ref"`
	AccessMode     string    `json:"access_mode"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type ApplicationView struct {
	ID                          uint                      `json:"id"`
	ProjectID                   uint                      `json:"project_id"`
	EnvironmentID               uint                      `json:"environment_id"`
	Name                        string                    `json:"name"`
	WorkloadKind                string                    `json:"workload_kind"`
	Capabilities                []string                  `json:"capabilities"`
	DefaultDeploymentTemplateID *uint                     `json:"default_deployment_template_id,omitempty"`
	CreatedBy                   uint                      `json:"created_by"`
	CreatedAt                   time.Time                 `json:"created_at"`
	UpdatedAt                   time.Time                 `json:"updated_at"`
	Project                     *ProjectView              `json:"project,omitempty"`
	Environment                 *EnvironmentView          `json:"environment,omitempty"`
	Endpoints                   []ApplicationEndpointView `json:"endpoints,omitempty"`
}

func EnvironmentDTO(e *model.Environment) *EnvironmentView {
	if e == nil {
		return nil
	}
	return &EnvironmentView{ID: e.ID, ProjectID: e.ProjectID, Name: e.Name, Namespace: e.Namespace, NamespaceStatus: e.NamespaceStatus, NamespaceConflict: e.NamespaceConflict, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}
func EnvironmentsDTO(items []model.Environment) []EnvironmentView {
	out := make([]EnvironmentView, 0, len(items))
	for i := range items {
		out = append(out, *EnvironmentDTO(&items[i]))
	}
	return out
}
func ProjectDTO(p *model.Project) *ProjectView {
	if p == nil {
		return nil
	}
	v := &ProjectView{ID: p.ID, Name: p.Name, Description: p.Description, DefaultImageRegistryID: p.DefaultImageRegistryID, OwnerID: p.OwnerID, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
	if len(p.Environments) > 0 {
		v.Environments = EnvironmentsDTO(p.Environments)
	}
	v.DefaultImageRegistry = ImageRegistryDTO(p.DefaultImageRegistry)
	return v
}
func ProjectsDTO(items []model.Project) []ProjectView {
	out := make([]ProjectView, 0, len(items))
	for i := range items {
		out = append(out, *ProjectDTO(&items[i]))
	}
	return out
}
func ApplicationEndpointDTO(e model.ApplicationEndpoint) ApplicationEndpointView {
	return ApplicationEndpointView{ID: e.ID, ApplicationID: e.ApplicationID, DomainID: e.DomainID, Exposure: e.Exposure, Domain: e.Domain, Path: e.Path, ServicePort: e.ServicePort, Protocol: e.Protocol, TLSEnabled: e.TLSEnabled, IngressEnabled: e.IngressEnabled, TLSSecretName: e.TLSSecretName, IssuerRef: e.IssuerRef, AccessMode: e.AccessMode, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}
func ApplicationDTO(a *model.Application) *ApplicationView {
	if a == nil {
		return nil
	}
	v := &ApplicationView{ID: a.ID, ProjectID: a.ProjectID, EnvironmentID: a.EnvironmentID, Name: a.Name, WorkloadKind: a.WorkloadKind, Capabilities: a.Capabilities, DefaultDeploymentTemplateID: a.DefaultDeploymentTemplateID, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
	if a.Project.ID != 0 {
		v.Project = ProjectDTO(&a.Project)
	}
	if a.Environment.ID != 0 {
		v.Environment = EnvironmentDTO(&a.Environment)
	}
	if len(a.Endpoints) > 0 {
		v.Endpoints = make([]ApplicationEndpointView, 0, len(a.Endpoints))
		for _, e := range a.Endpoints {
			v.Endpoints = append(v.Endpoints, ApplicationEndpointDTO(e))
		}
	}
	return v
}
func ApplicationsDTO(items []model.Application) []ApplicationView {
	out := make([]ApplicationView, 0, len(items))
	for i := range items {
		out = append(out, *ApplicationDTO(&items[i]))
	}
	return out
}

type ReleaseOperationView struct {
	ID          uint       `json:"id"`
	ReleaseID   uint       `json:"release_id"`
	Step        string     `json:"step"`
	Status      string     `json:"status"`
	Detail      string     `json:"detail"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
type ReleaseView struct {
	ID                 uint                   `json:"id"`
	ApplicationID      uint                   `json:"application_id"`
	Sequence           uint                   `json:"sequence"`
	Image              string                 `json:"image"`
	Version            string                 `json:"version,omitempty"`
	TemplateID         *uint                  `json:"template_id,omitempty"`
	TemplateRevision   uint                   `json:"template_revision,omitempty"`
	ImageDigest        string                 `json:"image_digest"`
	ImageRegistryID    *uint                  `json:"image_registry_id,omitempty"`
	PodTrackingEnabled bool                   `json:"pod_tracking_enabled"`
	DesiredSpec        string                 `json:"desired_spec"`
	Status             string                 `json:"status"`
	SourceReleaseID    *uint                  `json:"source_release_id,omitempty"`
	CreatedBy          uint                   `json:"created_by"`
	StartedAt          *time.Time             `json:"started_at,omitempty"`
	CompletedAt        *time.Time             `json:"completed_at,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	Operations         []ReleaseOperationView `json:"operations,omitempty"`
	Runtime            *ReleaseRuntimeView    `json:"runtime,omitempty"`
}

type ReleaseRuntimeView struct {
	Tracking   string                  `json:"tracking"`
	Pods       []ReleasePodRuntimeView `json:"pods"`
	Diagnostic string                  `json:"diagnostic,omitempty"`
}
type ReleasePodRuntimeView struct {
	Name       string                        `json:"name"`
	NodeName   string                        `json:"node_name,omitempty"`
	Phase      string                        `json:"phase"`
	Ready      bool                          `json:"ready"`
	Restarts   int32                         `json:"restarts"`
	CreatedAt  time.Time                     `json:"created_at"`
	Containers []ReleaseContainerRuntimeView `json:"containers"`
	Diagnostic string                        `json:"diagnostic,omitempty"`
}
type ReleaseContainerRuntimeView struct {
	Name         string `json:"name"`
	Image        string `json:"image,omitempty"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
	ExitCode     *int32 `json:"exit_code,omitempty"`
	LastState    string `json:"last_state,omitempty"`
	LastReason   string `json:"last_reason,omitempty"`
	LastExitCode *int32 `json:"last_exit_code,omitempty"`
}

func ReleaseDTO(r *model.Release) *ReleaseView {
	if r == nil {
		return nil
	}
	v := &ReleaseView{ID: r.ID, ApplicationID: r.ApplicationID, Sequence: r.Sequence, Image: r.Image, Version: r.Version, TemplateID: r.TemplateID, TemplateRevision: r.TemplateRevision, ImageDigest: r.ImageDigest, ImageRegistryID: r.ImageRegistryID, PodTrackingEnabled: r.PodTrackingEnabled, DesiredSpec: r.DesiredSpec, Status: r.Status, SourceReleaseID: r.SourceReleaseID, CreatedBy: r.CreatedBy, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Runtime: ReleaseRuntimeDTO(r.Runtime)}
	if len(r.Operations) > 0 {
		v.Operations = make([]ReleaseOperationView, 0, len(r.Operations))
		for _, o := range r.Operations {
			v.Operations = append(v.Operations, ReleaseOperationView{ID: o.ID, ReleaseID: o.ReleaseID, Step: o.Step, Status: o.Status, Detail: o.Detail, StartedAt: o.StartedAt, CompletedAt: o.CompletedAt, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt})
		}
	}
	return v
}

func ReleaseRuntimeDTO(r *model.ReleaseRuntime) *ReleaseRuntimeView {
	if r == nil {
		return nil
	}
	v := &ReleaseRuntimeView{Tracking: r.Tracking, Diagnostic: r.Diagnostic, Pods: make([]ReleasePodRuntimeView, 0, len(r.Pods))}
	for _, p := range r.Pods {
		pv := ReleasePodRuntimeView{Name: p.Name, NodeName: p.NodeName, Phase: p.Phase, Ready: p.Ready, Restarts: p.Restarts, CreatedAt: p.CreatedAt, Diagnostic: p.Diagnostic, Containers: make([]ReleaseContainerRuntimeView, 0, len(p.Containers))}
		for _, c := range p.Containers {
			pv.Containers = append(pv.Containers, ReleaseContainerRuntimeView{Name: c.Name, Image: c.Image, Ready: c.Ready, RestartCount: c.RestartCount, State: c.State, Reason: c.Reason, Message: c.Message, ExitCode: c.ExitCode, LastState: c.LastState, LastReason: c.LastReason, LastExitCode: c.LastExitCode})
		}
		v.Pods = append(v.Pods, pv)
	}
	return v
}
func ReleasesDTO(items []model.Release) []ReleaseView {
	out := make([]ReleaseView, 0, len(items))
	for i := range items {
		out = append(out, *ReleaseDTO(&items[i]))
	}
	return out
}

type RegistryProxyView struct {
	ID                      uint       `json:"id"`
	Name                    string     `json:"name"`
	Registry                string     `json:"registry"`
	UpstreamURL             string     `json:"upstream_url"`
	ResourceName            string     `json:"resource_name"`
	NodeName                string     `json:"node_name"`
	EndpointHost            string     `json:"endpoint_host"`
	NodePort                int32      `json:"node_port"`
	CacheLimitGi            int32      `json:"cache_limit_gi"`
	CleanupIntervalHours    int32      `json:"cleanup_interval_hours"`
	LastCleanupAt           *time.Time `json:"last_cleanup_at,omitempty"`
	LastCheckedAt           *time.Time `json:"last_checked_at,omitempty"`
	Status                  string     `json:"status"`
	LastError               string     `json:"last_error,omitempty"`
	NoProxy                 string     `json:"no_proxy,omitempty"`
	DNSServers              []string   `json:"dns_servers,omitempty"`
	LastDiagnosticStatus    string     `json:"last_diagnostic_status,omitempty"`
	LastDiagnosticError     string     `json:"last_diagnostic_error,omitempty"`
	LastDiagnosticAt        *time.Time `json:"last_diagnostic_at,omitempty"`
	OutboundProxyConfigured bool       `json:"outbound_proxy_configured"`
	CreatedBy               uint       `json:"created_by"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

// SiteView is the public representation of a managed site. Related server and
// certificate data are projected through their own views to avoid exposing
// persistence-only fields.
type SiteView struct {
	ID            uint        `json:"id"`
	ServerID      uint        `json:"server_id"`
	Domain        string      `json:"domain"`
	Port          int         `json:"port"`
	SSLEnabled    bool        `json:"ssl_enabled"`
	RootPath      string      `json:"root_path"`
	Managed       bool        `json:"managed"`
	NginxConfPath string      `json:"nginx_conf_path"`
	Upstream      string      `json:"upstream"`
	Locations     string      `json:"locations"`
	CertID        *uint       `json:"cert_id,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Server        *ServerView `json:"server,omitempty"`
	Cert          *CertView   `json:"cert,omitempty"`
}

type CertView struct {
	ID            uint       `json:"id"`
	SiteID        uint       `json:"site_id"`
	Domains       string     `json:"domains"`
	Provider      string     `json:"provider"`
	Account       string     `json:"account"`
	CertPath      string     `json:"cert_path"`
	KeyPath       string     `json:"key_path"`
	FullchainPath string     `json:"fullchain_path"`
	ValidFrom     time.Time  `json:"valid_from"`
	ValidTo       time.Time  `json:"valid_to"`
	Status        string     `json:"status"`
	Challenge     string     `json:"challenge"`
	LastRenew     *time.Time `json:"last_renew,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type DNSCredentialView struct {
	ID               uint      `json:"id"`
	Name             string    `json:"name"`
	Provider         string    `json:"provider"`
	Namespace        string    `json:"namespace"`
	SecretName       string    `json:"secret_name"`
	Enabled          bool      `json:"enabled"`
	CreatedBy        uint      `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	SecretConfigured bool      `json:"secret_configured"`
	ConfiguredFields []string  `json:"configured_fields,omitempty"`
}

type ChartRepositoryView struct {
	ID               uint       `json:"id"`
	Name             string     `json:"name"`
	Endpoint         string     `json:"endpoint"`
	ChartName        string     `json:"chart_name"`
	ChartVersion     string     `json:"chart_version"`
	Enabled          bool       `json:"enabled"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty"`
	LastVerifyStatus string     `json:"last_verify_status"`
	LastVerifyError  string     `json:"last_verify_error,omitempty"`
	CreatedBy        uint       `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type PlatformReleaseView struct {
	ID            uint       `json:"id"`
	Source        string     `json:"source"`
	Image         string     `json:"image"`
	PreviousImage string     `json:"previous_image"`
	Status        string     `json:"status"`
	Detail        string     `json:"detail"`
	CommitSHA     string     `json:"commit_sha,omitempty"`
	RunID         string     `json:"run_id,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type PersistentVolumeMigrationView struct {
	ID               uint       `json:"id"`
	EnvironmentID    uint       `json:"environment_id"`
	ApplicationID    uint       `json:"application_id"`
	SourcePVCName    string     `json:"source_pvc_name"`
	TargetPVCName    string     `json:"target_pvc_name"`
	SourceNodeName   string     `json:"source_node_name"`
	TargetNodeName   string     `json:"target_node_name"`
	SourceDeployment string     `json:"source_deployment"`
	SourceReplicas   int32      `json:"source_replicas"`
	Status           string     `json:"status"`
	Detail           string     `json:"detail"`
	BytesCopied      int64      `json:"bytes_copied"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type PersistentVolumeBackupView struct {
	ID                 uint       `json:"id"`
	EnvironmentID      uint       `json:"environment_id"`
	PVCName            string     `json:"pvc_name"`
	SourceNodeName     string     `json:"source_node_name"`
	BackupServerID     uint       `json:"backup_server_id"`
	BackupPath         string     `json:"backup_path"`
	Bytes              int64      `json:"bytes"`
	Status             string     `json:"status"`
	Detail             string     `json:"detail"`
	RestoreStatus      string     `json:"restore_status,omitempty"`
	RestoreDetail      string     `json:"restore_detail,omitempty"`
	CreatedBy          uint       `json:"created_by"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	RestoreStartedAt   *time.Time `json:"restore_started_at,omitempty"`
	RestoreCompletedAt *time.Time `json:"restore_completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type HostDirectoryPVCImportView struct {
	ID                   uint       `json:"id"`
	EnvironmentID        uint       `json:"environment_id"`
	PVCName              string     `json:"pvc_name"`
	SourceServerID       uint       `json:"source_server_id"`
	SourceNodeName       string     `json:"source_node_name"`
	SourcePath           string     `json:"source_path"`
	TargetNodeName       string     `json:"target_node_name"`
	TargetPath           string     `json:"target_path"`
	BackupPath           string     `json:"backup_path"`
	BackupChecksum       string     `json:"backup_checksum"`
	TargetBackupPath     string     `json:"target_backup_path,omitempty"`
	TargetBackupChecksum string     `json:"target_backup_checksum,omitempty"`
	SourceChecksum       string     `json:"source_checksum"`
	TargetChecksum       string     `json:"target_checksum"`
	BytesCopied          int64      `json:"bytes_copied"`
	Status               string     `json:"status"`
	Detail               string     `json:"detail"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
	BackupDeletedAt      *time.Time `json:"backup_deleted_at,omitempty"`
	CreatedBy            uint       `json:"created_by"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// PlatformEndpointView is the public representation of the manager entrypoint.
type PlatformEndpointView struct {
	ID              uint      `json:"id"`
	Hostname        string    `json:"hostname"`
	IngressName     string    `json:"ingress_name"`
	CertificateName string    `json:"certificate_name"`
	TLSSecretName   string    `json:"tls_secret_name"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AgentCapabilityGrantView struct {
	ID         uint   `json:"id"`
	RuntimeID  uint   `json:"runtime_id"`
	Capability string `json:"capability"`
	Namespace  string `json:"namespace"`
	Enabled    bool   `json:"enabled"`
	timeFields
}

type AlertAutomationPolicyView struct {
	ID              uint   `json:"id"`
	RuntimeID       uint   `json:"runtime_id"`
	Enabled         bool   `json:"enabled"`
	AlertName       string `json:"alert_name,omitempty"`
	MinimumSeverity string `json:"minimum_severity"`
	Mode            string `json:"mode"`
	CooldownMinutes int    `json:"cooldown_minutes"`
	timeFields
}

type AlertEventView struct {
	ID                uint       `json:"id"`
	Fingerprint       string     `json:"fingerprint"`
	AlertName         string     `json:"alert_name"`
	Severity          string     `json:"severity"`
	NodeName          string     `json:"node_name,omitempty"`
	MountPoint        string     `json:"mount_point,omitempty"`
	Labels            string     `json:"labels"`
	Annotations       string     `json:"annotations"`
	Status            string     `json:"status"`
	RuntimeID         *uint      `json:"runtime_id,omitempty"`
	SessionID         string     `json:"session_id,omitempty"`
	Report            string     `json:"report,omitempty"`
	DiagnosticSummary string     `json:"diagnostic_summary,omitempty"`
	OperationID       string     `json:"operation_id,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
	StartsAt          time.Time  `json:"starts_at"`
	EndsAt            *time.Time `json:"ends_at,omitempty"`
	LastDispatchedAt  *time.Time `json:"last_dispatched_at,omitempty"`
	timeFields
}

type AuditLogView struct {
	ID           uint      `json:"id"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   uint      `json:"resource_id"`
	UserID       uint      `json:"user_id"`
	Detail       string    `json:"detail"`
	CreatedAt    time.Time `json:"created_at"`
}

type OperationLogView struct {
	ID           uint      `json:"id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   uint      `json:"resource_id"`
	Step         string    `json:"step"`
	Status       string    `json:"status"`
	Detail       string    `json:"detail"`
	CreatedAt    time.Time `json:"created_at"`
}

type DashboardStatsView struct {
	TotalServers  int64 `json:"total_servers"`
	TotalSites    int64 `json:"total_sites"`
	ExpiringCerts int64 `json:"expiring_certs"`
	ExpiredCerts  int64 `json:"expired_certs"`
}

type timeFields struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func PlatformEndpointDTO(e *model.PlatformEndpoint) *PlatformEndpointView {
	if e == nil {
		return nil
	}
	return &PlatformEndpointView{ID: e.ID, Hostname: e.Hostname, IngressName: e.IngressName, CertificateName: e.CertificateName, TLSSecretName: e.TLSSecretName, Enabled: e.Enabled, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}

func AgentCapabilityGrantDTO(g model.AgentCapabilityGrant) AgentCapabilityGrantView {
	return AgentCapabilityGrantView{ID: g.ID, RuntimeID: g.RuntimeID, Capability: g.Capability, Namespace: g.Namespace, Enabled: g.Enabled, timeFields: timeFields{CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt}}
}
func AgentCapabilityGrantsDTO(items []model.AgentCapabilityGrant) []AgentCapabilityGrantView {
	out := make([]AgentCapabilityGrantView, 0, len(items))
	for _, g := range items {
		out = append(out, AgentCapabilityGrantDTO(g))
	}
	return out
}
func AlertAutomationPolicyDTO(p *model.AlertAutomationPolicy) *AlertAutomationPolicyView {
	if p == nil {
		return nil
	}
	return &AlertAutomationPolicyView{ID: p.ID, RuntimeID: p.RuntimeID, Enabled: p.Enabled, AlertName: p.AlertName, MinimumSeverity: p.MinimumSeverity, Mode: p.Mode, CooldownMinutes: p.CooldownMinutes, timeFields: timeFields{CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}}
}
func AlertEventDTO(e model.AlertEvent) AlertEventView {
	return AlertEventView{ID: e.ID, Fingerprint: e.Fingerprint, AlertName: e.AlertName, Severity: e.Severity, NodeName: e.NodeName, MountPoint: e.MountPoint, Labels: e.Labels, Annotations: e.Annotations, Status: e.Status, RuntimeID: e.RuntimeID, SessionID: e.SessionID, Report: e.Report, DiagnosticSummary: e.DiagnosticSummary, OperationID: e.OperationID, LastError: e.LastError, StartsAt: e.StartsAt, EndsAt: e.EndsAt, LastDispatchedAt: e.LastDispatchedAt, timeFields: timeFields{CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}}
}
func AlertEventsDTO(items []model.AlertEvent) []AlertEventView {
	out := make([]AlertEventView, 0, len(items))
	for _, e := range items {
		out = append(out, AlertEventDTO(e))
	}
	return out
}
func AuditLogDTO(log model.AuditLog) AuditLogView {
	return AuditLogView{ID: log.ID, Action: log.Action, ResourceType: log.ResourceType, ResourceID: log.ResourceID, UserID: log.UserID, Detail: log.Detail, CreatedAt: log.CreatedAt}
}
func AuditLogsDTO(items []model.AuditLog) []AuditLogView {
	out := make([]AuditLogView, 0, len(items))
	for _, log := range items {
		out = append(out, AuditLogDTO(log))
	}
	return out
}
func OperationLogDTO(log model.OperationLog) OperationLogView {
	return OperationLogView{ID: log.ID, ResourceType: log.ResourceType, ResourceID: log.ResourceID, Step: log.Step, Status: log.Status, Detail: log.Detail, CreatedAt: log.CreatedAt}
}
func OperationLogsDTO(items []model.OperationLog) []OperationLogView {
	out := make([]OperationLogView, 0, len(items))
	for _, log := range items {
		out = append(out, OperationLogDTO(log))
	}
	return out
}
func DashboardStatsDTO(stats *model.DashboardStats) *DashboardStatsView {
	if stats == nil {
		return nil
	}
	return &DashboardStatsView{TotalServers: stats.TotalServers, TotalSites: stats.TotalSites, ExpiringCerts: stats.ExpiringCerts, ExpiredCerts: stats.ExpiredCerts}
}

func RegistryProxyDTO(p *model.RegistryProxy) *RegistryProxyView {
	if p == nil {
		return nil
	}
	return &RegistryProxyView{ID: p.ID, Name: p.Name, Registry: p.Registry, UpstreamURL: p.UpstreamURL, ResourceName: p.ResourceName, NodeName: p.NodeName, EndpointHost: p.EndpointHost, NodePort: p.NodePort, CacheLimitGi: p.CacheLimitGi, CleanupIntervalHours: p.CleanupIntervalHours, LastCleanupAt: p.LastCleanupAt, LastCheckedAt: p.LastCheckedAt, Status: p.Status, LastError: p.LastError, NoProxy: p.NoProxy, DNSServers: p.DNSServers, LastDiagnosticStatus: p.LastDiagnosticStatus, LastDiagnosticError: p.LastDiagnosticError, LastDiagnosticAt: p.LastDiagnosticAt, OutboundProxyConfigured: p.OutboundProxyConfigured, CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

func SiteDTO(s *model.Site) *SiteView {
	if s == nil {
		return nil
	}
	v := &SiteView{ID: s.ID, ServerID: s.ServerID, Domain: s.Domain, Port: s.Port, SSLEnabled: s.SSLEnabled, RootPath: s.RootPath, Managed: s.Managed, NginxConfPath: s.NginxConfPath, Upstream: s.Upstream, Locations: s.Locations, CertID: s.CertID, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	if s.Server.ID != 0 {
		v.Server = ServerDTO(&s.Server)
	}
	v.Cert = CertDTO(s.Cert)
	return v
}
func SitesDTO(items []model.Site) []SiteView {
	out := make([]SiteView, 0, len(items))
	for i := range items {
		out = append(out, *SiteDTO(&items[i]))
	}
	return out
}
func CertDTO(c *model.Cert) *CertView {
	if c == nil {
		return nil
	}
	return &CertView{ID: c.ID, SiteID: c.SiteID, Domains: c.Domains, Provider: c.Provider, Account: c.Account, CertPath: c.CertPath, KeyPath: c.KeyPath, FullchainPath: c.FullchainPath, ValidFrom: c.ValidFrom, ValidTo: c.ValidTo, Status: c.Status, Challenge: c.Challenge, LastRenew: c.LastRenew, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}
func CertsDTO(items []model.Cert) []CertView {
	out := make([]CertView, 0, len(items))
	for i := range items {
		out = append(out, *CertDTO(&items[i]))
	}
	return out
}
func DNSCredentialDTO(c *model.DNSCredential) *DNSCredentialView {
	if c == nil {
		return nil
	}
	return &DNSCredentialView{ID: c.ID, Name: c.Name, Provider: c.Provider, Namespace: c.Namespace, SecretName: c.SecretName, Enabled: c.Enabled, CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, SecretConfigured: c.SecretConfigured, ConfiguredFields: c.ConfiguredFields}
}
func DNSCredentialsDTO(items []model.DNSCredential) []DNSCredentialView {
	out := make([]DNSCredentialView, 0, len(items))
	for i := range items {
		out = append(out, *DNSCredentialDTO(&items[i]))
	}
	return out
}
func ChartRepositoryDTO(c *model.ChartRepository) *ChartRepositoryView {
	if c == nil {
		return nil
	}
	return &ChartRepositoryView{ID: c.ID, Name: c.Name, Endpoint: c.Endpoint, ChartName: c.ChartName, ChartVersion: c.ChartVersion, Enabled: c.Enabled, LastVerifiedAt: c.LastVerifiedAt, LastVerifyStatus: c.LastVerifyStatus, LastVerifyError: c.LastVerifyError, CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}
func ChartRepositoriesDTO(items []model.ChartRepository) []ChartRepositoryView {
	out := make([]ChartRepositoryView, 0, len(items))
	for i := range items {
		out = append(out, *ChartRepositoryDTO(&items[i]))
	}
	return out
}
func PlatformReleaseDTO(r *model.PlatformRelease) *PlatformReleaseView {
	if r == nil {
		return nil
	}
	return &PlatformReleaseView{ID: r.ID, Source: r.Source, Image: r.Image, PreviousImage: r.PreviousImage, Status: r.Status, Detail: r.Detail, CommitSHA: r.CommitSHA, RunID: r.RunID, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func PlatformReleasesDTO(items []model.PlatformRelease) []PlatformReleaseView {
	out := make([]PlatformReleaseView, 0, len(items))
	for i := range items {
		out = append(out, *PlatformReleaseDTO(&items[i]))
	}
	return out
}
func PersistentVolumeMigrationDTO(m *model.PersistentVolumeMigration) *PersistentVolumeMigrationView {
	if m == nil {
		return nil
	}
	return &PersistentVolumeMigrationView{ID: m.ID, EnvironmentID: m.EnvironmentID, ApplicationID: m.ApplicationID, SourcePVCName: m.SourcePVCName, TargetPVCName: m.TargetPVCName, SourceNodeName: m.SourceNodeName, TargetNodeName: m.TargetNodeName, SourceDeployment: m.SourceDeployment, SourceReplicas: m.SourceReplicas, Status: m.Status, Detail: m.Detail, BytesCopied: m.BytesCopied, StartedAt: m.StartedAt, CompletedAt: m.CompletedAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func PersistentVolumeMigrationsDTO(items []model.PersistentVolumeMigration) []PersistentVolumeMigrationView {
	out := make([]PersistentVolumeMigrationView, 0, len(items))
	for i := range items {
		out = append(out, *PersistentVolumeMigrationDTO(&items[i]))
	}
	return out
}
func PersistentVolumeBackupDTO(b *model.PersistentVolumeBackup) *PersistentVolumeBackupView {
	if b == nil {
		return nil
	}
	return &PersistentVolumeBackupView{ID: b.ID, EnvironmentID: b.EnvironmentID, PVCName: b.PVCName, SourceNodeName: b.SourceNodeName, BackupServerID: b.BackupServerID, BackupPath: b.BackupPath, Bytes: b.Bytes, Status: b.Status, Detail: b.Detail, RestoreStatus: b.RestoreStatus, RestoreDetail: b.RestoreDetail, CreatedBy: b.CreatedBy, StartedAt: b.StartedAt, CompletedAt: b.CompletedAt, RestoreStartedAt: b.RestoreStartedAt, RestoreCompletedAt: b.RestoreCompletedAt, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
}
func PersistentVolumeBackupsDTO(items []model.PersistentVolumeBackup) []PersistentVolumeBackupView {
	out := make([]PersistentVolumeBackupView, 0, len(items))
	for i := range items {
		out = append(out, *PersistentVolumeBackupDTO(&items[i]))
	}
	return out
}
func HostDirectoryPVCImportDTO(t *model.HostDirectoryPVCImport) *HostDirectoryPVCImportView {
	if t == nil {
		return nil
	}
	return &HostDirectoryPVCImportView{ID: t.ID, EnvironmentID: t.EnvironmentID, PVCName: t.PVCName, SourceServerID: t.SourceServerID, SourceNodeName: t.SourceNodeName, SourcePath: t.SourcePath, TargetNodeName: t.TargetNodeName, TargetPath: t.TargetPath, BackupPath: t.BackupPath, BackupChecksum: t.BackupChecksum, TargetBackupPath: t.TargetBackupPath, TargetBackupChecksum: t.TargetBackupChecksum, SourceChecksum: t.SourceChecksum, TargetChecksum: t.TargetChecksum, BytesCopied: t.BytesCopied, Status: t.Status, Detail: t.Detail, VerifiedAt: t.VerifiedAt, BackupDeletedAt: t.BackupDeletedAt, CreatedBy: t.CreatedBy, StartedAt: t.StartedAt, CompletedAt: t.CompletedAt, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}
func HostDirectoryPVCImportsDTO(items []model.HostDirectoryPVCImport) []HostDirectoryPVCImportView {
	out := make([]HostDirectoryPVCImportView, 0, len(items))
	for i := range items {
		out = append(out, *HostDirectoryPVCImportDTO(&items[i]))
	}
	return out
}
