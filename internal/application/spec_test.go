package application

import (
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestValidateReleaseSpec(t *testing.T) {
	spec := ReleaseSpec{
		Image:         "registry.example.com/order-api:1.0.0",
		ContainerPort: 8080,
		Replicas:      2,
		Resources: ResourceSpec{
			RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi",
		},
		Health:   HealthSpec{ReadinessPath: "/healthz", LivenessPath: "/healthz"},
		Service:  ServiceSpec{Port: 80, TargetPort: 8080},
		Endpoint: EndpointSpec{Exposure: ExposurePublic, Domain: "api.example.com", TLSEnabled: true, IssuerRef: "letsencrypt-prod"},
	}
	if issues := ValidateReleaseSpec(spec); len(issues) != 0 {
		t.Fatalf("expected valid spec, got %+v", issues)
	}

	spec.Endpoint.Domain = ""
	issues := ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[0].Field != "endpoint.domain" {
		t.Fatalf("expected public domain validation error, got %+v", issues)
	}
}

func TestRenderResourcesUsesManagedLabelsAndRedactsSecret(t *testing.T) {
	spec := ReleaseSpec{
		Image:         "nginx:1.27",
		Command:       []string{"/docker-entrypoint.sh"},
		Args:          []string{"nginx", "-g", "daemon off;"},
		ContainerPort: 8080,
		Replicas:      2,
		Resources:     ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"},
		Health:        HealthSpec{ReadinessPath: "/ready", LivenessPath: "/live"},
		Service:       ServiceSpec{Port: 80, TargetPort: 8080},
		Config:        map[string]string{"LOG_LEVEL": "info"},
		Secrets:       map[string]string{"DATABASE_PASSWORD": "not-for-snapshot"},
		Endpoint:      EndpointSpec{Exposure: ExposureCluster},
	}
	resources, err := RenderResources(ApplicationContext{
		ProjectID: 7, EnvironmentID: 11, ProjectName: "前端", EnvironmentName: "生产环境", ApplicationName: "order-api", Namespace: "commerce-prod", ReleaseSequence: 3,
	}, spec)
	if err != nil {
		t.Fatalf("RenderResources: %v", err)
	}
	if resources.Deployment.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("expected managed label, got %+v", resources.Deployment.Labels)
	}
	if resources.Deployment.Spec.Template.Labels[ReleaseLabel] != "3" || resources.Deployment.Annotations[ReleaseLabel] != "3" {
		t.Fatalf("release sequence must label the Pod template: %#v", resources.Deployment)
	}
	if resources.Deployment.Spec.Selector.MatchLabels[ReleaseLabel] != "" {
		t.Fatalf("release sequence must not mutate the Deployment selector: %#v", resources.Deployment.Spec.Selector)
	}
	container := resources.Deployment.Spec.Template.Spec.Containers[0]
	if got := container.Command; len(got) != 1 || got[0] != "/docker-entrypoint.sh" {
		t.Fatalf("expected command to reach the container, got %#v", got)
	}
	if got := container.Args; len(got) != 3 || got[2] != "daemon off;" {
		t.Fatalf("expected args to reach the container, got %#v", got)
	}
	if resources.Deployment.Labels[ProjectLabel] != "project-7" || resources.Deployment.Labels[EnvironmentLabel] != "environment-11" {
		t.Fatalf("expected ID-based project and environment labels, got %+v", resources.Deployment.Labels)
	}
	if resources.Service.Spec.Selector[ApplicationNameLabel] != "order-api" {
		t.Fatalf("unexpected selector: %+v", resources.Service.Spec.Selector)
	}
	if resources.Secret == nil || string(resources.Secret.StringData["DATABASE_PASSWORD"]) != "not-for-snapshot" {
		t.Fatal("expected secret to remain in the generated Kubernetes resource")
	}
	if got := resources.SanitizedSpec.Secrets["DATABASE_PASSWORD"]; got != "" {
		t.Fatalf("expected secret snapshot to omit values, got %q", got)
	}
	if len(resources.Deployment.Spec.Template.Spec.Containers[0].EnvFrom) != 2 {
		t.Fatal("expected config and secret env references")
	}
}

func TestRenderResourcesSkipsDisabledConfigAndSecretEntries(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Config = map[string]string{"HYSTERIA_LOG_LEVEL": "debug", "config.yaml": "listen: :8443"}
	spec.ConfigDisabled = []string{"HYSTERIA_LOG_LEVEL"}
	spec.Secrets = map[string]string{"ACTIVE": "yes", "DISABLED": "no"}
	spec.SecretsDisabled = []string{"DISABLED"}
	resources, err := RenderResources(ApplicationContext{ProjectID: 1, EnvironmentID: 1, ProjectName: "p", EnvironmentName: "e", ApplicationName: "app", Namespace: "ns", ReleaseSequence: 1}, spec)
	if err != nil {
		t.Fatalf("RenderResources: %v", err)
	}
	if resources.ConfigMap == nil || resources.ConfigMap.Data["HYSTERIA_LOG_LEVEL"] != "" {
		t.Fatalf("disabled ConfigMap key was rendered: %#v", resources.ConfigMap)
	}
	if resources.ConfigMap.Data["config.yaml"] == "" {
		t.Fatal("enabled ConfigMap key was omitted")
	}
	if resources.Secret == nil || resources.Secret.StringData["DISABLED"] != "" || resources.Secret.StringData["ACTIVE"] != "yes" {
		t.Fatalf("unexpected rendered Secret: %#v", resources.Secret)
	}
}

func TestRenderResourcesRendersUDPServiceAndFileMounts(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.ContainerPort = 443
	spec.Service = ServiceSpec{
		Port:                  443,
		TargetPort:            443,
		Protocol:              ServiceProtocolUDP,
		Type:                  ServiceTypeLoadBalancer,
		ExternalTrafficPolicy: string(corev1.ServiceExternalTrafficPolicyLocal),
	}
	spec.FileMounts = []FileMountSpec{
		{SourceType: FileMountSourceSecret, SourceName: "edge-tls", Key: "tls.crt", MountPath: "/run/app/tls/tls.crt"},
		{SourceType: FileMountSourceSecret, SourceName: "edge-tls", Key: "tls.key", MountPath: "/run/app/tls/tls.key"},
		{SourceType: FileMountSourceConfigMap, SourceName: "edge-config", Key: "config.yaml", MountPath: "/etc/app/config.yaml"},
	}
	resources, err := RenderResources(ApplicationContext{ProjectName: "edge", EnvironmentName: "production", ApplicationName: "udp-server", Namespace: "edge-prod", ReleaseSequence: 1}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if resources.Service.Spec.Type != corev1.ServiceTypeLoadBalancer || resources.Service.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
		t.Fatalf("expected UDP LoadBalancer Service, got %#v", resources.Service.Spec)
	}
	if port := resources.Service.Spec.Ports[0]; port.Protocol != corev1.ProtocolUDP || port.Port != 443 || port.TargetPort.IntVal != 443 {
		t.Fatalf("expected UDP Service port, got %#v", port)
	}
	container := resources.Deployment.Spec.Template.Spec.Containers[0]
	if port := container.Ports[0]; port.Protocol != corev1.ProtocolUDP || port.ContainerPort != 443 {
		t.Fatalf("expected UDP container port, got %#v", port)
	}
	if len(resources.Deployment.Spec.Template.Spec.Volumes) != 2 || len(container.VolumeMounts) != 2 {
		t.Fatalf("expected grouped projected volumes, got volumes=%#v mounts=%#v", resources.Deployment.Spec.Template.Spec.Volumes, container.VolumeMounts)
	}
	tlsVolume := resources.Deployment.Spec.Template.Spec.Volumes[0]
	if tlsVolume.Secret == nil || tlsVolume.Secret.SecretName != "edge-tls" || len(tlsVolume.Secret.Items) != 2 {
		t.Fatalf("expected projected TLS Secret, got %#v", tlsVolume)
	}
	if container.VolumeMounts[0].MountPath != "/run/app/tls" || !container.VolumeMounts[0].ReadOnly {
		t.Fatalf("expected read-only TLS directory mount, got %#v", container.VolumeMounts[0])
	}
}

func TestRenderResourcesUsesHostNetworkWithRecreateDeploymentStrategy(t *testing.T) {
	spec := validTestReleaseSpec()
	defaultResources, err := RenderResources(ApplicationContext{ProjectName: "edge", EnvironmentName: "production", ApplicationName: "default", Namespace: "edge-prod", ReleaseSequence: 1}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if defaultResources.Deployment.Spec.Strategy.Type != "" || defaultResources.Deployment.Spec.Strategy.RollingUpdate != nil {
		t.Fatalf("non-host-network deployment should retain the default strategy, got %#v", defaultResources.Deployment.Spec.Strategy)
	}
	spec.HostNetwork = true
	resources, err := RenderResources(ApplicationContext{ProjectName: "edge", EnvironmentName: "production", ApplicationName: "hysteria", Namespace: "edge-prod", ReleaseSequence: 1}, spec)
	if err != nil {
		t.Fatal(err)
	}
	podSpec := resources.Deployment.Spec.Template.Spec
	if !podSpec.HostNetwork || podSpec.DNSPolicy != corev1.DNSClusterFirstWithHostNet {
		t.Fatalf("expected host-network Pod with cluster DNS, got %#v", podSpec)
	}
	if resources.Deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType || resources.Deployment.Spec.Strategy.RollingUpdate != nil {
		t.Fatalf("expected Recreate strategy for host-network deployment, got %#v", resources.Deployment.Spec.Strategy)
	}
}

func TestRenderResourcesMountsApplicationConfigKeyAsFile(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Config = map[string]string{"config.yaml": "listen: :8080"}
	spec.FileMounts = []FileMountSpec{{
		SourceType: FileMountSourceApplicationConfig,
		Key:        "config.yaml",
		MountPath:  "/etc/app/config.yaml",
	}}
	resources, err := RenderResources(ApplicationContext{ProjectName: "edge", EnvironmentName: "production", ApplicationName: "edge-api", Namespace: "edge-prod", ReleaseSequence: 1}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if resources.ConfigMap == nil || resources.ConfigMap.Name != "edge-api-config" {
		t.Fatalf("expected generated application ConfigMap, got %#v", resources.ConfigMap)
	}
	volume := resources.Deployment.Spec.Template.Spec.Volumes[0]
	if volume.ConfigMap == nil || volume.ConfigMap.Name != "edge-api-config" || len(volume.ConfigMap.Items) != 1 || volume.ConfigMap.Items[0].Key != "config.yaml" {
		t.Fatalf("expected projected application ConfigMap, got %#v", volume)
	}
	if mount := resources.Deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0]; mount.MountPath != "/etc/app" || !mount.ReadOnly {
		t.Fatalf("expected read-only application config mount, got %#v", mount)
	}
}

func TestRenderResourcesMountsApplicationSecretKeyAsFile(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Secrets = map[string]string{"config.yaml": "listen: :8443"}
	spec.FileMounts = []FileMountSpec{{
		SourceType: FileMountSourceApplicationSecret,
		Key:        "config.yaml",
		MountPath:  "/etc/app/config.yaml",
		Managed:    true,
	}}
	resources, err := RenderResources(ApplicationContext{ProjectName: "edge", EnvironmentName: "production", ApplicationName: "edge-api", Namespace: "edge-prod", ReleaseSequence: 1}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if resources.Secret == nil || resources.Secret.Name != "edge-api-secret" {
		t.Fatalf("expected generated application Secret, got %#v", resources.Secret)
	}
	volume := resources.Deployment.Spec.Template.Spec.Volumes[0]
	if volume.Secret == nil || volume.Secret.SecretName != "edge-api-secret" || len(volume.Secret.Items) != 1 || volume.Secret.Items[0].Key != "config.yaml" {
		t.Fatalf("expected projected application Secret, got %#v", volume)
	}
}

func TestRenderResourcesRendersMultiProtocolServicePorts(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Service = ServiceSpec{
		Type:                  ServiceTypeLoadBalancer,
		ExternalTrafficPolicy: string(corev1.ServiceExternalTrafficPolicyLocal),
		Ports: []ServicePortSpec{
			{Name: "proxy", Port: 443, TargetPort: 443, Protocol: ServiceProtocolUDP, NodePort: 30443},
			{Name: "api", Port: 8080, TargetPort: 8080, Protocol: ServiceProtocolTCP, NodePort: 30080},
		},
	}
	spec.Endpoint = EndpointSpec{Exposure: ExposurePublic, Domain: "api.example.com"}

	resources, err := RenderResources(ApplicationContext{ProjectName: "edge", EnvironmentName: "production", ApplicationName: "multi-port", Namespace: "edge-prod", ReleaseSequence: 1}, spec)
	if err != nil {
		t.Fatal(err)
	}
	ports := resources.Service.Spec.Ports
	if len(ports) != 2 || ports[0].Name != "proxy" || ports[0].Protocol != corev1.ProtocolUDP || ports[0].NodePort != 30443 || ports[1].Name != "api" || ports[1].Protocol != corev1.ProtocolTCP || ports[1].NodePort != 30080 {
		t.Fatalf("expected TCP and UDP Service ports, got %#v", ports)
	}
	containerPorts := resources.Deployment.Spec.Template.Spec.Containers[0].Ports
	if len(containerPorts) != 2 || containerPorts[0].ContainerPort != 443 || containerPorts[0].Protocol != corev1.ProtocolUDP || containerPorts[1].ContainerPort != 8080 || containerPorts[1].Protocol != corev1.ProtocolTCP {
		t.Fatalf("expected matching container ports, got %#v", containerPorts)
	}
	if resources.Ingress == nil || resources.Ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Number != 8080 {
		t.Fatalf("expected Ingress to select the first TCP Service port, got %#v", resources.Ingress)
	}
}

func TestValidateReleaseSpecRejectsInvalidMultiPortService(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Service = ServiceSpec{Ports: []ServicePortSpec{
		{Name: "proxy", Port: 443, TargetPort: 443, Protocol: ServiceProtocolUDP},
		{Name: "proxy", Port: 8080, TargetPort: 8080, Protocol: ServiceProtocolTCP},
	}}
	issues := ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[len(issues)-1].Field != "service.ports[1].name" {
		t.Fatalf("expected duplicate Service port name error, got %#v", issues)
	}

	spec = validTestReleaseSpec()
	spec.Service = ServiceSpec{Ports: []ServicePortSpec{{Name: "proxy", Port: 443, TargetPort: 443, Protocol: ServiceProtocolUDP}}}
	spec.Endpoint = EndpointSpec{Exposure: ExposurePublic, Domain: "udp.example.com"}
	issues = ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[0].Field != "endpoint.exposure" {
		t.Fatalf("expected UDP-only Service to reject HTTP Ingress, got %#v", issues)
	}
}

func TestValidateReleaseSpecRejectsInvalidL4ServiceAndFileMounts(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Service = ServiceSpec{Port: 80, TargetPort: 8080, Type: ServiceTypeClusterIP, NodePort: 30443}
	issues := ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[len(issues)-1].Field != "service.node_port" {
		t.Fatalf("expected ClusterIP node port validation error, got %#v", issues)
	}

	spec = validTestReleaseSpec()
	spec.FileMounts = []FileMountSpec{{SourceType: FileMountSourceSecret, SourceName: "valid-name", Key: "tls.key", MountPath: "relative/key"}}
	issues = ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[len(issues)-1].Field != "file_mounts[0].mount_path" {
		t.Fatalf("expected unsafe file mount validation error, got %#v", issues)
	}

	spec = validTestReleaseSpec()
	spec.FileMounts = []FileMountSpec{{SourceType: FileMountSourceApplicationConfig, Key: "config.yaml", MountPath: "/etc/app/config.yaml"}}
	issues = ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[0].Field != "file_mounts[0].key" {
		t.Fatalf("expected missing application ConfigMap key error, got %#v", issues)
	}

	spec = validTestReleaseSpec()
	spec.FileMounts = []FileMountSpec{{SourceType: FileMountSourceSecret, SourceName: "edge-tls", Key: "tls.crt", MountPath: "/etc/app/tls.crt", Managed: true}}
	issues = ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[0].Field != "file_mounts[0].managed" {
		t.Fatalf("expected managed mount source validation error, got %#v", issues)
	}

	spec = validTestReleaseSpec()
	spec.Service.Protocol = ServiceProtocolUDP
	spec.Endpoint = EndpointSpec{Exposure: ExposurePublic, Domain: "udp.example.com"}
	issues = ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[0].Field != "endpoint.exposure" {
		t.Fatalf("expected UDP HTTP Ingress validation error, got %#v", issues)
	}
}

func TestRenderResourcesReusesManagedDomainCertificate(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Endpoint = EndpointSpec{Exposure: ExposurePublic, DomainID: 7, Domain: "api.example.com", TLSEnabled: true, IssuerRef: "letsencrypt-prod", IssuerKind: "ClusterIssuer", ManagedCertificateName: "cylism-domain-7", ManagedTLSSecretName: "cylism-domain-7-tls"}
	resources, err := RenderResources(ApplicationContext{ProjectName: "commerce", EnvironmentName: "production", ApplicationName: "order-api", Namespace: "commerce-prod", ReleaseSequence: 4}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if resources.Certificate != nil {
		t.Fatal("managed domain must not render another Certificate")
	}
	if resources.Ingress == nil || len(resources.Ingress.Spec.TLS) != 1 || resources.Ingress.Spec.TLS[0].SecretName != "cylism-domain-7-tls" {
		t.Fatalf("expected Ingress to reuse managed TLS Secret: %#v", resources.Ingress)
	}
}

func TestRenderResourcesRejectsInvalidApplicationName(t *testing.T) {
	_, err := RenderResources(ApplicationContext{ProjectName: "commerce", EnvironmentName: "production", ApplicationName: "订单服务", Namespace: "commerce-prod", ReleaseSequence: 1}, validTestReleaseSpec())
	if err == nil || err.Error() != "应用名称必须为 1-63 位小写字母、数字或连字符，且以字母或数字开头和结尾" {
		t.Fatalf("expected Kubernetes application name validation error, got %v", err)
	}
}

func TestRenderResourcesUsesSecretReferenceWhenSnapshotIsRedacted(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Secrets = map[string]string{"DATABASE_PASSWORD": ""}
	resources, err := RenderResources(ApplicationContext{ProjectName: "commerce", EnvironmentName: "production", ApplicationName: "order-api", Namespace: "commerce-prod", ReleaseSequence: 4}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if resources.Secret != nil {
		t.Fatal("redacted snapshot must not overwrite the existing Secret")
	}
	if len(resources.Deployment.Spec.Template.Spec.Containers[0].EnvFrom) != 1 {
		t.Fatal("expected the existing Secret to remain referenced")
	}
}

func TestValidateReleaseSpecRejectsRequestsAboveLimits(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Resources.RequestsCPU = "600m"
	spec.Resources.LimitsCPU = "500m"
	issues := ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[0].Field != "resources.requests_cpu" {
		t.Fatalf("expected CPU request limit validation, got %+v", issues)
	}
}

func TestRenderResourcesSupportsOptionalAndTCPHealthChecks(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Health = HealthSpec{ReadinessEnabled: true, ReadinessType: "tcp", LivenessEnabled: false}
	resources, err := RenderResources(ApplicationContext{ProjectName: "commerce", EnvironmentName: "production", ApplicationName: "order-api", Namespace: "commerce-prod", ReleaseSequence: 4}, spec)
	if err != nil {
		t.Fatal(err)
	}
	container := resources.Deployment.Spec.Template.Spec.Containers[0]
	if container.ReadinessProbe == nil || container.ReadinessProbe.TCPSocket == nil || container.LivenessProbe != nil {
		t.Fatalf("unexpected health probes: %+v", container)
	}
}

func TestRenderResourcesDoesNotCreateDisabledHealthChecks(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Health = HealthSpec{
		ReadinessEnabled: false,
		ReadinessType:    "http",
		ReadinessPath:    "/healthz",
		LivenessEnabled:  false,
		LivenessType:     "http",
		LivenessPath:     "/healthz",
	}
	resources, err := RenderResources(ApplicationContext{ProjectName: "commerce", EnvironmentName: "production", ApplicationName: "order-api", Namespace: "commerce-prod", ReleaseSequence: 4}, spec)
	if err != nil {
		t.Fatal(err)
	}
	container := resources.Deployment.Spec.Template.Spec.Containers[0]
	if container.ReadinessProbe != nil || container.LivenessProbe != nil {
		t.Fatalf("disabled health checks must not render probes: %+v", container)
	}
}

func TestRenderResourcesMountsPersistentVolumeClaimsOnSelectedNode(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Replicas = 1
	spec.NodeName = "storage-node-a"
	spec.Volumes = []VolumeMountSpec{{ClaimName: "karakeep-data", MountPath: "/data"}}
	resources, err := RenderResources(ApplicationContext{ProjectName: "knowledge", EnvironmentName: "production", ApplicationName: "karakeep", Namespace: "project-knowledge-prod", ReleaseSequence: 4}, spec)
	if err != nil {
		t.Fatal(err)
	}
	podSpec := resources.Deployment.Spec.Template.Spec
	if podSpec.NodeName != "" || podSpec.NodeSelector["kubernetes.io/hostname"] != "storage-node-a" || len(podSpec.Volumes) != 1 || podSpec.Volumes[0].PersistentVolumeClaim == nil || podSpec.Volumes[0].PersistentVolumeClaim.ClaimName != "karakeep-data" {
		t.Fatalf("expected PVC volume and node placement, got %#v", podSpec)
	}
	container := podSpec.Containers[0]
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].MountPath != "/data" || container.VolumeMounts[0].ReadOnly {
		t.Fatalf("expected writable /data mount, got %#v", container.VolumeMounts)
	}
	if resources.Deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Fatalf("expected Recreate strategy for PVC deployment, got %#v", resources.Deployment.Spec.Strategy)
	}
}

func TestRenderResourcesCreatesStatefulSetForStatefulApplication(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Replicas = 1
	spec.Volumes = []VolumeMountSpec{{ClaimName: "meilisearch-data", MountPath: "/meili_data"}}
	resources, err := RenderResources(ApplicationContext{ProjectName: "knowledge", EnvironmentName: "production", ApplicationName: "meilisearch", Namespace: "project-knowledge", ReleaseSequence: 8, WorkloadKind: WorkloadKindStatefulSet}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if resources.Deployment != nil || resources.StatefulSet == nil {
		t.Fatalf("expected only a StatefulSet, got deployment=%#v statefulset=%#v", resources.Deployment, resources.StatefulSet)
	}
	if resources.StatefulSet.Spec.ServiceName != "meilisearch" || resources.StatefulSet.Spec.Template.Labels[ReleaseLabel] != "8" {
		t.Fatalf("unexpected StatefulSet identity: %#v", resources.StatefulSet)
	}
	podSpec := resources.StatefulSet.Spec.Template.Spec
	if len(podSpec.Volumes) != 1 || podSpec.Volumes[0].PersistentVolumeClaim == nil || podSpec.Volumes[0].PersistentVolumeClaim.ClaimName != "meilisearch-data" {
		t.Fatalf("expected StatefulSet to reuse PVC, got %#v", podSpec.Volumes)
	}
}

func TestValidateReleaseSpecRejectsReplicatedPersistentVolumeClaims(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Replicas = 2
	spec.Volumes = []VolumeMountSpec{{ClaimName: "karakeep-data", MountPath: "/data"}}
	issues := ValidateReleaseSpec(spec)
	if len(issues) == 0 || issues[len(issues)-1].Field != "volumes" {
		t.Fatalf("expected PVC replica validation issue, got %#v", issues)
	}
}

func TestRenderResourcesAddsImagePullSecretForPrivateRegistry(t *testing.T) {
	spec := validTestReleaseSpec()
	spec.Image = "harbor.example.com/commerce/order-api:1.0.0"
	spec.RegistryID = 12
	spec.RegistryEndpoint = "harbor.example.com"
	spec.RegistryAuthType = "basic"
	spec.RegistryUsername = "robot$commerce"
	spec.RegistryCredential = "registry-password"

	resources, err := RenderResources(ApplicationContext{ProjectName: "commerce", EnvironmentName: "production", ApplicationName: "order-api", Namespace: "commerce-prod", ReleaseSequence: 5}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if resources.ImagePullSecret == nil || resources.ImagePullSecret.Name != "cylism-regcred-12" {
		t.Fatalf("expected registry pull secret, got %+v", resources.ImagePullSecret)
	}
	if resources.ImagePullSecret.Type != "kubernetes.io/dockerconfigjson" {
		t.Fatalf("unexpected pull secret type: %s", resources.ImagePullSecret.Type)
	}
	var dockerConfig map[string]map[string]map[string]string
	if err := json.Unmarshal(resources.ImagePullSecret.Data[".dockerconfigjson"], &dockerConfig); err != nil {
		t.Fatalf("decode docker config: %v", err)
	}
	if dockerConfig["auths"]["harbor.example.com"]["username"] != "robot$commerce" {
		t.Fatalf("unexpected docker config: %+v", dockerConfig)
	}
	if got := resources.Deployment.Spec.Template.Spec.ImagePullSecrets; len(got) != 1 || got[0].Name != "cylism-regcred-12" {
		t.Fatalf("expected deployment image pull secret, got %+v", got)
	}
	if resources.SanitizedSpec.RegistryCredential != "" || resources.SanitizedSpec.RegistryUsername != "" || resources.SanitizedSpec.RegistryEndpoint != "" {
		t.Fatalf("registry credentials must not be stored in the snapshot: %+v", resources.SanitizedSpec)
	}
}
