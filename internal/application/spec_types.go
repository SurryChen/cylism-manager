package application

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	ManagedByLabel       = "app.kubernetes.io/managed-by"
	ManagedByValue       = "cylism-manager"
	ProjectLabel         = "app.kubernetes.io/part-of"
	ApplicationNameLabel = "app.kubernetes.io/name"
	EnvironmentLabel     = "cylism.io/environment"
	ReleaseLabel         = "cylism.io/release"

	ExposureCluster = "cluster"
	ExposureTailnet = "tailnet"
	ExposurePublic  = "public"

	WorkloadKindDeployment  = "deployment"
	WorkloadKindStatefulSet = "statefulset"

	ServiceProtocolTCP = "TCP"
	ServiceProtocolUDP = "UDP"

	ServiceTypeClusterIP    = "ClusterIP"
	ServiceTypeNodePort     = "NodePort"
	ServiceTypeLoadBalancer = "LoadBalancer"

	FileMountSourceConfigMap = "configmap"
	FileMountSourceSecret    = "secret"
	// FileMountSourceApplicationConfig projects a key from the ConfigMap
	// generated for this application's template config.
	FileMountSourceApplicationConfig = "application_config"
	FileMountSourceApplicationSecret = "application_secret"
)

type ReleaseSpec struct {
	Image      string   `json:"image"`
	Version    string   `json:"version,omitempty"`
	Command    []string `json:"command,omitempty"`
	Args       []string `json:"args,omitempty"`
	RegistryID uint     `json:"registry_id,omitempty"`
	// 以下字段只在发布执行期存在，禁止写入 API 响应或发布快照。
	RegistryEndpoint                    string            `json:"-"`
	RegistryAuthType                    string            `json:"-"`
	RegistryUsername                    string            `json:"-"`
	RegistryCredential                  string            `json:"-"`
	ImageVerificationEndpoint           string            `json:"-"`
	ImageVerificationUsername           string            `json:"-"`
	ImageVerificationCredential         string            `json:"-"`
	ImageVerificationInsecureSkipVerify bool              `json:"-"`
	ContainerPort                       int32             `json:"container_port"`
	Replicas                            int32             `json:"replicas"`
	Resources                           ResourceSpec      `json:"resources"`
	Health                              HealthSpec        `json:"health"`
	Config                              map[string]string `json:"config,omitempty"`
	// ConfigDisabled keeps configured values in the template while preventing
	// selected keys from being projected into the workload. Missing entries are enabled
	// for backwards compatibility with existing templates.
	ConfigDisabled []string `json:"config_disabled,omitempty"`
	// ConfigManagedKeys lists ConfigMap keys that an external integration may edit.
	// An empty list only falls back to file_mounts[].managed for legacy templates.
	ConfigManagedKeys []string          `json:"config_managed_keys,omitempty"`
	Secrets           map[string]string `json:"secrets,omitempty"`
	SecretsDisabled   []string          `json:"secrets_disabled,omitempty"`
	// SecretManagedKeys lists Secret keys that an external integration may edit.
	SecretManagedKeys []string          `json:"secret_managed_keys,omitempty"`
	NodeName          string            `json:"node_name,omitempty"`
	HostNetwork       bool              `json:"host_network,omitempty"`
	Volumes           []VolumeMountSpec `json:"volumes,omitempty"`
	FileMounts        []FileMountSpec   `json:"file_mounts,omitempty"`
	Service           ServiceSpec       `json:"service"`
	Endpoint          EndpointSpec      `json:"endpoint"`
}

// VolumeMountSpec describes a platform-managed PVC mounted into the main
// application container. PVC lifecycle remains independent from releases.

type VolumeMountSpec struct {
	ClaimName string `json:"claim_name"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only"`
}

// FileMountSpec projects one ConfigMap or Secret key at an absolute path in
// the primary container. Application config mounts resolve to the ConfigMap
// generated for the current release. Projections are always read-only.

type FileMountSpec struct {
	SourceType string `json:"source_type"`
	SourceName string `json:"source_name"`
	Key        string `json:"key"`
	MountPath  string `json:"mount_path"`
	Managed    bool   `json:"managed,omitempty"`
}

type ResourceSpec struct {
	RequestsCPU    string `json:"requests_cpu"`
	RequestsMemory string `json:"requests_memory"`
	LimitsCPU      string `json:"limits_cpu"`
	LimitsMemory   string `json:"limits_memory"`
}

type HealthSpec struct {
	ReadinessEnabled bool   `json:"readiness_enabled"`
	ReadinessType    string `json:"readiness_type,omitempty"`
	ReadinessPath    string `json:"readiness_path,omitempty"`
	LivenessEnabled  bool   `json:"liveness_enabled"`
	LivenessType     string `json:"liveness_type,omitempty"`
	LivenessPath     string `json:"liveness_path,omitempty"`
}

type ServiceSpec struct {
	Port                  int32             `json:"port"`
	TargetPort            int32             `json:"target_port"`
	Protocol              string            `json:"protocol,omitempty"`
	Type                  string            `json:"type,omitempty"`
	NodePort              int32             `json:"node_port,omitempty"`
	ExternalTrafficPolicy string            `json:"external_traffic_policy,omitempty"`
	Ports                 []ServicePortSpec `json:"ports,omitempty"`
}

// ServicePortSpec describes one port of a Kubernetes Service. The Service
// type and external traffic policy remain shared by all declared ports.

type ServicePortSpec struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort int32  `json:"target_port"`
	Protocol   string `json:"protocol"`
	NodePort   int32  `json:"node_port,omitempty"`
}

// PortSpecs converts legacy single-port Service fields to one named port.
// Non-empty Ports always take precedence so a template has one source of
// truth after it is migrated to the multi-port representation.

func (spec ServiceSpec) PortSpecs() []ServicePortSpec {
	if len(spec.Ports) > 0 {
		return append([]ServicePortSpec(nil), spec.Ports...)
	}
	return []ServicePortSpec{{Name: "service", Port: spec.Port, TargetPort: spec.TargetPort, Protocol: spec.Protocol, NodePort: spec.NodePort}}
}

func (spec ServiceSpec) PrimaryTCPPort() (ServicePortSpec, bool) {
	for _, port := range spec.PortSpecs() {
		if normalizeServiceProtocol(port.Protocol) == ServiceProtocolTCP {
			return port, true
		}
	}
	return ServicePortSpec{}, false
}

type EndpointSpec struct {
	Exposure               string `json:"exposure"`
	DomainID               uint   `json:"domain_id,omitempty"`
	Domain                 string `json:"domain,omitempty"`
	Path                   string `json:"path,omitempty"`
	TLSEnabled             bool   `json:"tls_enabled"`
	IssuerRef              string `json:"issuer_ref,omitempty"`
	IssuerKind             string `json:"issuer_kind,omitempty"`
	ManagedCertificateName string `json:"managed_certificate_name,omitempty"`
	ManagedTLSSecretName   string `json:"managed_tls_secret_name,omitempty"`
}

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ApplicationContext struct {
	ProjectID       uint
	EnvironmentID   uint
	ProjectName     string
	EnvironmentName string
	ApplicationName string
	Namespace       string
	ReleaseSequence uint
	WorkloadKind    string
}

type RenderedResources struct {
	ConfigMap       *corev1.ConfigMap
	Secret          *corev1.Secret
	ImagePullSecret *corev1.Secret
	Deployment      *appsv1.Deployment
	StatefulSet     *appsv1.StatefulSet
	Service         *corev1.Service
	Certificate     *unstructured.Unstructured
	Ingress         *networkingv1.Ingress
	SanitizedSpec   ReleaseSpec
}
