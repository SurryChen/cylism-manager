package cloud

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestStaticAssetUploadSignature(t *testing.T) {
	provider := &aliyunProvider{region: "cn-guangzhou", accessKeyID: "test-key", accessKeySecret: "test-secret"}
	signed, err := provider.SignStaticAssetUpload("test-assets", "pending/example.png", "image/png", 1234, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Host != "test-assets.oss-cn-guangzhou.aliyuncs.com" || parsed.Path != "/pending/example.png" ||
		query.Get("x-oss-signature-version") != "OSS4-HMAC-SHA256" ||
		!strings.Contains(query.Get("x-oss-additional-headers"), "content-length") ||
		query.Get("x-oss-signature") == "" {
		t.Fatalf("unexpected signed upload URL: %s", signed)
	}
}
