package cloud

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestStaticAssetUploadSignature(t *testing.T) {
	provider := &aliyunProvider{region: "cn-guangzhou", accessKeyID: "test-key", accessKeySecret: "test-secret"}
	endpoint, fields, err := provider.SignStaticAssetUpload("test-assets", "pending/example.png", "image/png", 1234, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := base64.StdEncoding.DecodeString(fields["policy"])
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://test-assets.cn-guangzhou.oss.aliyuncs.com/" ||
		fields["x-oss-signature-version"] != "OSS4-HMAC-SHA256" || len(fields["x-oss-signature"]) != 64 ||
		!strings.Contains(string(policy), `["content-length-range",1234,1234]`) ||
		!strings.Contains(string(policy), `"key":"pending/example.png"`) {
		t.Fatalf("unexpected OSS upload policy: %s", policy)
	}
}
