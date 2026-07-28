package application

import "testing"

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
		ProjectName: "commerce", EnvironmentName: "production", ApplicationName: "order-api", Namespace: "commerce-prod", ReleaseSequence: 3,
	}, spec)
	if err != nil {
		t.Fatalf("RenderResources: %v", err)
	}
	if resources.Deployment.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("expected managed label, got %+v", resources.Deployment.Labels)
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
