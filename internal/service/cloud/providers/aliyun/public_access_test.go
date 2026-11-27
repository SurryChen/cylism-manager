package cloud

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
)

func TestBucketPublicAccessUsesBucketSubresource(t *testing.T) {
	updated := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["publicAccessBlock"]; !ok || !strings.Contains(r.URL.Path, "/test-assets") ||
			!strings.HasPrefix(r.Header.Get("Authorization"), "OSS4-HMAC-SHA256 ") {
			t.Errorf("unexpected OSS request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte("<PublicAccessBlockConfiguration><BlockPublicAccess>true</BlockPublicAccess></PublicAccessBlockConfiguration>"))
		case http.MethodPut:
			var body ossPublicAccessBlock
			if err := xml.NewDecoder(r.Body).Decode(&body); err != nil || body.BlockPublicAccess == nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			updated <- *body.BlockPublicAccess
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client, err := oss.New(server.URL, "test-key", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	provider := &aliyunProvider{oss: client, region: "cn-guangzhou", accessKeyID: "test-key", accessKeySecret: "test-secret"}

	config, err := provider.GetBucketPublicAccess(context.Background(), "test-assets")
	if err != nil || !config.BlockPublicAccess {
		t.Fatalf("GET bucket public access: config=%+v err=%v", config, err)
	}
	if err := provider.SetBucketPublicAccess(context.Background(), "test-assets", cloudservice.BucketPublicAccess{BlockPublicAccess: false}); err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || <-updated {
		t.Fatal("PUT did not send an explicit false")
	}
}
