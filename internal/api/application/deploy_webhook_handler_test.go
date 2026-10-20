package applicationapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestApplicationDeployWebhookSignsDefaultTemplateRelease(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "application-webhook.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := createApplicationForReleaseRuntimeTest(t, s)
	spec := applicationservice.ReleaseSpec{Image: "ghcr.io/example/browser", ContainerPort: 8080, Replicas: 1, Resources: applicationservice.ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"}, Service: applicationservice.ServiceSpec{Port: 80, TargetPort: 8080}}
	snapshot, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Enabled: true, Spec: string(snapshot)}
	if err := s.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: app.Environment.Namespace}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}})}
	h := newTestApplicationHandler(s, []byte("01234567890123456789012345678901"), NewKubernetesAdapter(client))
	r := gin.New()
	r.GET("/api/applications/:id/deploy-webhook", h.DeployWebhookStatus)
	r.POST("/api/applications/:id/deploy-webhook/secret", h.GenerateDeployWebhookSecret)
	r.DELETE("/api/applications/:id/deploy-webhook", h.DisableDeployWebhook)
	r.POST("/api/applications/:id/deployments", h.DeployWebhook)

	path := fmt.Sprintf("/api/applications/%d/deployments", app.ID)
	if response := serve(r, newJSONRequest(http.MethodPost, path, gin.H{"version": "1.2.3"})); response.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned callback = %d: %s", response.Code, response.Body.String())
	}
	generated := serve(r, newJSONRequest(http.MethodPost, fmt.Sprintf("/api/applications/%d/deploy-webhook/secret", app.ID), nil))
	if generated.Code != http.StatusOK {
		t.Fatalf("generate secret = %d: %s", generated.Code, generated.Body.String())
	}
	var secretResponse struct {
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(generated.Body.Bytes(), &secretResponse); err != nil || len(secretResponse.Data.Secret) != 64 {
		t.Fatalf("invalid generated secret: %v", err)
	}
	status := serve(r, newJSONRequest(http.MethodGet, fmt.Sprintf("/api/applications/%d/deploy-webhook", app.ID), nil))
	if !strings.Contains(status.Body.String(), `"configured":true`) || strings.Contains(status.Body.String(), secretResponse.Data.Secret) {
		t.Fatalf("status exposes secret or omits enabled state: %s", status.Body.String())
	}

	body := `{"version":"1.2.3"}`
	timestamp := time.Now().Unix()
	nonce := "webhook-nonce-123456"
	for _, invalid := range []string{`{"version":"bad tag"}`, `{"version":"1.2.3","image":"attacker/image"}`} {
		response := serve(r, signedApplicationWebhookRequest(path, invalid, timestamp, "invalid-body-nonce-123456", secretResponse.Data.Secret))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid body accepted: %d: %s", response.Code, response.Body.String())
		}
	}
	request := signedApplicationWebhookRequest(path, body, timestamp, nonce, secretResponse.Data.Secret)
	response := serve(r, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"version":"1.2.3"`) {
		t.Fatalf("signed callback = %d: %s", response.Code, response.Body.String())
	}
	releases, err := s.ListReleases(app.ID)
	if err != nil || len(releases) != 1 || releases[0].Image != "ghcr.io/example/browser:1.2.3" || releases[0].TemplateID == nil || *releases[0].TemplateID != template.ID {
		t.Fatalf("unexpected releases: %#v, err=%v", releases, err)
	}
	if replay := serve(r, signedApplicationWebhookRequest(path, body, timestamp, nonce, secretResponse.Data.Secret)); replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay = %d: %s", replay.Code, replay.Body.String())
	}
	if bad := serve(r, signedApplicationWebhookRequest(path, body, timestamp, "another-nonce-123456", "wrong-secret")); bad.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret = %d: %s", bad.Code, bad.Body.String())
	}
	if old := serve(r, signedApplicationWebhookRequest(path, body, timestamp-600, "expired-nonce-123456", secretResponse.Data.Secret)); old.Code != http.StatusUnauthorized {
		t.Fatalf("expired request = %d: %s", old.Code, old.Body.String())
	}
	rotated := serve(r, newJSONRequest(http.MethodPost, fmt.Sprintf("/api/applications/%d/deploy-webhook/secret", app.ID), nil))
	if rotated.Code != http.StatusOK || strings.Contains(rotated.Body.String(), secretResponse.Data.Secret) {
		t.Fatalf("rotation failed: %d: %s", rotated.Code, rotated.Body.String())
	}
	if stale := serve(r, signedApplicationWebhookRequest(path, body, timestamp, "rotated-nonce-123456", secretResponse.Data.Secret)); stale.Code != http.StatusUnauthorized {
		t.Fatalf("rotated secret still valid = %d: %s", stale.Code, stale.Body.String())
	}
	if disabled := serve(r, newJSONRequest(http.MethodDelete, fmt.Sprintf("/api/applications/%d/deploy-webhook", app.ID), nil)); disabled.Code != http.StatusOK {
		t.Fatalf("disable webhook = %d: %s", disabled.Code, disabled.Body.String())
	}
	if afterDisable := serve(r, signedApplicationWebhookRequest(path, body, timestamp, "disabled-nonce-123456", secretResponse.Data.Secret)); afterDisable.Code != http.StatusUnauthorized {
		t.Fatalf("disabled callback = %d: %s", afterDisable.Code, afterDisable.Body.String())
	}
}

func signedApplicationWebhookRequest(path, body string, timestamp int64, nonce, secret string) *http.Request {
	stamp := strconv.FormatInt(timestamp, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stamp + "." + nonce + "." + body))
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Cylism-Timestamp", stamp)
	request.Header.Set("X-Cylism-Nonce", nonce)
	request.Header.Set("X-Cylism-Signature", hex.EncodeToString(mac.Sum(nil)))
	return request
}
