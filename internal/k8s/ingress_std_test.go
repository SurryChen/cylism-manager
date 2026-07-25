package k8s

import (
	"testing"
)

func TestIngressStdInfo_FieldsComplete(t *testing.T) {
	i := IngressStdInfo{
		Name:      "web-ingress",
		Namespace: "default",
		Hosts:     []string{"example.com"},
		Paths:     []string{"/ -> web-svc:80"},
		TLS:       []string{"tls-secret"},
		Controller: "traefik",
		Age:        "3d",
	}
	if i.Controller != "traefik" {
		t.Errorf("expected traefik controller, got %s", i.Controller)
	}
}

func TestIngressControllerStatus_FieldsComplete(t *testing.T) {
	c := IngressControllerStatus{
		Type:    "Traefik",
		Version: "2.10.7",
		Running: true,
		CRD:     true,
		Namespace: "kube-system",
	}
	if !c.Running {
		t.Error("expected controller to be running")
	}
}

func TestIngressStdMethods_Exist(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	_ = client.ListIngresses
	_ = client.GetIngress
	_ = client.CreateIngress
	_ = client.DeleteIngress
	_ = client.DetectIngressController
}
