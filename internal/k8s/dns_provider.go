package k8s

import (
	"fmt"
	"sort"
	"strings"
)

// DNSProviderField describes one credential input accepted by a controlled provider adapter.
type DNSProviderField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
}

// DNSProviderInfo is safe for API responses and intentionally excludes provider secret values.
type DNSProviderInfo struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Fields      []DNSProviderField `json:"fields"`
	Webhook     bool               `json:"webhook"`
	Description string             `json:"description"`
}

type dnsWebhookChart struct {
	ReleaseName string
	Repository  string
	Chart       string
	Version     string
	Values      string
}

// DNSProviderAdapter is a compiled, reviewed DNS-01 integration. It never accepts raw Helm or solver input.
type DNSProviderAdapter interface {
	Info() DNSProviderInfo
	ValidateValues(map[string]string) error
	SecretData(map[string]string) map[string]string
	Solver(secretName string) map[string]interface{}
	CredentialSecretName(solver map[string]interface{}) string
	WebhookChart() *dnsWebhookChart
}

var dnsProviders = map[string]DNSProviderAdapter{
	"alidns": aliDNSProvider{},
}

func GetDNSProvider(id string) (DNSProviderAdapter, bool) {
	provider, ok := dnsProviders[strings.ToLower(strings.TrimSpace(id))]
	return provider, ok
}

func ListDNSProviders() []DNSProviderInfo {
	providers := make([]DNSProviderInfo, 0, len(dnsProviders))
	for _, provider := range dnsProviders {
		providers = append(providers, provider.Info())
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })
	return providers
}

type aliDNSProvider struct{}

func (aliDNSProvider) Info() DNSProviderInfo {
	return DNSProviderInfo{ID: "alidns", Name: "阿里云 DNS", Description: "通过 AliDNS DNS-01 Webhook 申请证书", Webhook: true, Fields: []DNSProviderField{{Key: "access_key_id", Label: "AccessKey ID", Required: true}, {Key: "access_key_secret", Label: "AccessKey Secret", Secret: true, Required: true}}}
}
func (aliDNSProvider) ValidateValues(values map[string]string) error {
	for _, key := range []string{"access_key_id", "access_key_secret"} {
		if strings.TrimSpace(values[key]) == "" {
			return fmt.Errorf("AliDNS %s 必填", key)
		}
	}
	return nil
}
func (aliDNSProvider) SecretData(values map[string]string) map[string]string {
	return map[string]string{"access-key-id": values["access_key_id"], "access-key-secret": values["access_key_secret"]}
}
func (aliDNSProvider) Solver(secretName string) map[string]interface{} {
	return map[string]interface{}{"dns01": map[string]interface{}{"webhook": map[string]interface{}{"groupName": "acme.cylism.io", "solverName": "alidns", "config": map[string]interface{}{"region": "cn-hangzhou", "accessKeyIdRef": map[string]interface{}{"name": secretName, "key": "access-key-id"}, "accessKeySecretRef": map[string]interface{}{"name": secretName, "key": "access-key-secret"}}}}}
}
func (aliDNSProvider) CredentialSecretName(solver map[string]interface{}) string {
	name, _, _ := nestedStringMap(solver, "dns01", "webhook", "config", "accessKeyIdRef", "name")
	return name
}
func (aliDNSProvider) WebhookChart() *dnsWebhookChart {
	return &dnsWebhookChart{ReleaseName: "cylism-alidns-webhook", Repository: "https://wjiec.github.io/alidns-webhook", Chart: "alidns-webhook", Version: "1.0.3", Values: "groupName: acme.cylism.io\n"}
}

func nestedStringMap(value map[string]interface{}, fields ...string) (string, bool, error) {
	current := value
	for index, field := range fields {
		next, ok := current[field]
		if !ok {
			return "", false, nil
		}
		if index == len(fields)-1 {
			text, ok := next.(string)
			return text, ok, nil
		}
		current, ok = next.(map[string]interface{})
		if !ok {
			return "", false, fmt.Errorf("field %s is not an object", field)
		}
	}
	return "", false, nil
}
