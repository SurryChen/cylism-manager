package cloud

import "testing"

func TestNewProviderDoesNotRequireAccountRegion(t *testing.T) {
	provider, err := NewProvider(`{"access_key_id":"id","access_key_secret":"secret"}`, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if provider.(*aliyunProvider).region != "cn-hangzhou" {
		t.Fatalf("unexpected fallback storage region: %q", provider.(*aliyunProvider).region)
	}
}

func TestStorageRegionCanBeSelectedPerRequest(t *testing.T) {
	provider, err := NewProvider(`{"access_key_id":"id","access_key_secret":"secret"}`, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	aliyun := provider.(*aliyunProvider)
	if err := aliyun.SetStorageRegion("cn-shanghai"); err != nil {
		t.Fatal(err)
	}
	if aliyun.region != "cn-shanghai" {
		t.Fatalf("storage region = %q", aliyun.region)
	}
	if err := aliyun.SetStorageRegion("cn-hangzhou.evil.example"); err == nil {
		t.Fatal("invalid region must not become an OSS endpoint")
	}
}

func TestBucketLocationUsesRegionIdentifier(t *testing.T) {
	if got := bucketRegion("", "oss-cn-hangzhou"); got != "cn-hangzhou" {
		t.Fatalf("bucket region = %q", got)
	}
}
