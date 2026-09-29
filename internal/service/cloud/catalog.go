package cloud

// ProviderField describes one safe, user-facing connection field. Secret
// values are accepted by the API only as part of the encrypted credential
// payload and are never returned in this catalog.
type ProviderField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Placeholder string `json:"placeholder,omitempty"`
	Required    bool   `json:"required"`
}

// ProviderSpec is the configuration contract shown in System Settings.
type ProviderSpec struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	Implemented         bool            `json:"implemented"`
	Capabilities        []string        `json:"capabilities"`
	CredentialFields    []ProviderField `json:"credential_fields"`
	ConfigurationFields []ProviderField `json:"configuration_fields"`
}

var providerSpecs = []ProviderSpec{
	{
		ID: "aliyun", Name: "阿里云", Description: "Alibaba Cloud DNS 与 OSS",
		Capabilities: []string{"dns", "object_storage"},
		CredentialFields: []ProviderField{
			{Key: "access_key_id", Label: "AccessKey ID", Type: "text", Required: true},
			{Key: "access_key_secret", Label: "AccessKey Secret", Type: "password", Required: true},
		},
		ConfigurationFields: []ProviderField{{Key: "region", Label: "默认地域", Type: "text", Placeholder: "例如 cn-hangzhou", Required: true}},
	},
	{
		ID: "tencent", Name: "腾讯云", Description: "Tencent Cloud DNSPod 与 COS",
		Capabilities: []string{"dns", "object_storage"},
		CredentialFields: []ProviderField{
			{Key: "secret_id", Label: "SecretId", Type: "text", Required: true},
			{Key: "secret_key", Label: "SecretKey", Type: "password", Required: true},
		},
		ConfigurationFields: []ProviderField{{Key: "region", Label: "默认地域", Type: "text", Placeholder: "例如 ap-guangzhou", Required: true}},
	},
	{
		ID: "cloudcone", Name: "CloudCone", Description: "CloudCone API 与 S3 兼容对象存储",
		Capabilities:        []string{"object_storage"},
		CredentialFields:    []ProviderField{{Key: "api_key", Label: "API Key", Type: "password", Required: true}},
		ConfigurationFields: []ProviderField{{Key: "endpoint", Label: "API Endpoint", Type: "url", Placeholder: "https://api.cloudcone.com", Required: true}},
	},
}

// ProviderCatalog returns a copy so callers cannot mutate the registry's
// shared metadata. Implemented is derived from the adapters actually loaded.
func ProviderCatalog(registry ProviderRegistry) []ProviderSpec {
	result := make([]ProviderSpec, len(providerSpecs))
	for i, spec := range providerSpecs {
		result[i] = spec
		result[i].Implemented = false
		if _, ok := registry[spec.ID]; ok {
			result[i].Implemented = true
		}
		result[i].Capabilities = append([]string(nil), spec.Capabilities...)
		result[i].CredentialFields = append([]ProviderField(nil), spec.CredentialFields...)
		result[i].ConfigurationFields = append([]ProviderField(nil), spec.ConfigurationFields...)
	}
	return result
}
