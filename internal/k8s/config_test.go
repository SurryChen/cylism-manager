package k8s

import (
	"testing"
)

func TestConfigMapInfo_FieldsComplete(t *testing.T) {
	c := ConfigMapInfo{
		Name:      "app-config",
		Namespace: "default",
		Keys:      []string{"PORT", "LOG_LEVEL"},
		KeysCount: 2,
		UsedBy:    []WorkloadRef{{Kind: "Deployment", Name: "web-app", RefType: "env"}},
		Age:       "12d",
	}
	if c.KeysCount != 2 {
		t.Errorf("expected 2 keys, got %d", c.KeysCount)
	}
}

func TestSecretInfo_FieldsComplete(t *testing.T) {
	s := SecretInfo{
		Name:      "db-password",
		Namespace: "default",
		Type:      "Opaque",
		Keys:      []string{"password"},
		KeysCount: 1,
		UsedBy:    []WorkloadRef{{Kind: "Deployment", Name: "api-server", RefType: "volume"}},
		Age:       "5d",
	}
	if s.Type != "Opaque" {
		t.Errorf("expected Opaque type, got %s", s.Type)
	}
}

func TestSecretDetail_FieldsComplete(t *testing.T) {
	s := SecretDetail{
		Name:      "tls-cert",
		Namespace: "default",
		Type:      "kubernetes.io/tls",
		Data:      map[string]string{"tls.crt": "base64encoded...", "tls.key": "base64encoded..."},
		UsedBy:    []WorkloadRef{},
		Age:       "30d",
	}
	if len(s.Data) != 2 {
		t.Errorf("expected 2 data keys, got %d", len(s.Data))
	}
}

func TestConfigMethods_Exist(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	_ = client.ListConfigMaps
	_ = client.GetConfigMap
	_ = client.ListSecrets
	_ = client.GetSecret
}
