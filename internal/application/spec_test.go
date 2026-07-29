package application

import (
	"encoding/json"
	"testing"
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
