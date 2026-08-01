package application

import (
	"testing"
)

func TestValidateStackSpecAcceptsIndependentApplications(t *testing.T) {
	spec := StackSpec{
		EntryApplication: "karakeep",
		Components: []StackComponent{
			{ApplicationName: "karakeep-meilisearch", Spec: stackTestSpec("meili", 7700)},
			{ApplicationName: "karakeep", Spec: stackTestSpec("karakeep", 3000)},
		},
	}
	if issues := ValidateStackSpec(spec); len(issues) > 0 {
		t.Fatalf("expected independent applications to be valid: %#v", issues)
	}
}

func TestValidateStackSpecRequiresMainComponent(t *testing.T) {
	issues := ValidateStackSpec(StackSpec{EntryApplication: "karakeep", Components: []StackComponent{{ApplicationName: "karakeep-meilisearch", Spec: stackTestSpec("meili", 7700)}}})
	if len(issues) == 0 {
		t.Fatal("expected missing main component validation error")
	}
}

func stackTestSpec(image string, port int32) ReleaseSpec {
	return ReleaseSpec{
		Image: image, ContainerPort: port, Replicas: 1,
		Resources: ResourceSpec{RequestsCPU: "10m", RequestsMemory: "16Mi", LimitsCPU: "100m", LimitsMemory: "64Mi"},
		Health:    HealthSpec{}, Service: ServiceSpec{Port: port, TargetPort: port}, Endpoint: EndpointSpec{Exposure: ExposureCluster},
	}
}
