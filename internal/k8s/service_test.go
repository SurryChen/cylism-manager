package k8s

import (
	"testing"
)

func TestServiceEndpointInfo_FieldsComplete(t *testing.T) {
	s := ServiceEndpointInfo{
		Name:           "myservice",
		Namespace:      "default",
		Type:           "ClusterIP",
		ClusterIP:      "10.43.1.25",
		Ports:          []string{"TCP:80"},
		EndpointCount:  3,
		Selector:       map[string]string{"app": "web"},
		Age:            "5d",
	}
	if s.EndpointCount != 3 {
		t.Errorf("expected 3 endpoints, got %d", s.EndpointCount)
	}
}

func TestEndpointSliceInfo_FieldsComplete(t *testing.T) {
	slice := EndpointSliceInfo{
		Name:        "myservice-abc12",
		Namespace:   "default",
		AddressType: "IPv4",
		Endpoints: []EndpointPoint{
			{IP: "10.42.0.5", Node: "node-1", PodName: "web-abc123", Ready: true},
			{IP: "10.42.0.6", Node: "node-2", PodName: "web-def456", Ready: false},
		},
	}
	if len(slice.Endpoints) != 2 {
		t.Errorf("expected 2 endpoint points, got %d", len(slice.Endpoints))
	}
	if slice.Endpoints[0].Ready != true {
		t.Error("expected first endpoint to be ready")
	}
}

func TestServiceMethods_Exist(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	_ = client.ListServices
	_ = client.GetService
	_ = client.GetServiceEndpoints
}
