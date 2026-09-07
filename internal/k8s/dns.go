package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// UpsertDNSCredentialSecret writes one controlled provider Secret. Values must already pass adapter validation.
func (c *Client) UpsertDNSCredentialSecretContext(ctx context.Context, providerID, namespace, name string, values map[string]string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	provider, ok := GetDNSProvider(providerID)
	if !ok {
		return fmt.Errorf("不支持的 DNS Provider: %s", providerID)
	}
	if err := provider.ValidateValues(values); err != nil {
		return err
	}
	data := provider.SecretData(values)
	secrets := c.Clientset.CoreV1().Secrets(namespace)
	current, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = secrets.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "cylism.io/dns-provider": providerID}}, Type: corev1.SecretTypeOpaque, StringData: data}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	current.StringData = data
	if current.Labels == nil {
		current.Labels = map[string]string{}
	}
	current.Labels["app.kubernetes.io/managed-by"] = "cylism-manager"
	current.Labels["cylism.io/dns-provider"] = providerID
	_, err = secrets.Update(ctx, current, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteDNSCredentialSecretContext(ctx context.Context, namespace, name string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	err := c.Clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
