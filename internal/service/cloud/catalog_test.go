package cloud

import "testing"

func TestProviderCatalogIncludesCommonProviders(t *testing.T) {
	catalog := ProviderCatalog(ProviderRegistry{"aliyun": func(string, string) (Provider, error) { return nil, nil }})
	byID := make(map[string]ProviderSpec, len(catalog))
	for _, item := range catalog {
		byID[item.ID] = item
	}
	for _, id := range []string{"aliyun", "tencent", "cloudcone"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("provider catalog missing %q", id)
		}
	}
	if !byID["aliyun"].Implemented || len(byID["aliyun"].CredentialFields) == 0 {
		t.Fatalf("aliyun catalog entry = %#v", byID["aliyun"])
	}
	if len(byID["aliyun"].ConfigurationFields) != 0 {
		t.Fatalf("account catalog must not expose storage region: %#v", byID["aliyun"].ConfigurationFields)
	}
	if byID["tencent"].Implemented || byID["cloudcone"].Implemented {
		t.Fatal("unregistered providers must be marked unavailable")
	}
}
