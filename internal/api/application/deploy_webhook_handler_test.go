package applicationapi

import (
	"context"
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
	crypto "github.com/cylism/cylism-manager/internal/security"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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

func TestMigrationWebhookAndPreflightContract(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "migration-webhook.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := createApplicationForReleaseRuntimeTest(t, st)
	key := []byte("01234567890123456789012345678901")
	spec := applicationservice.ReleaseSpec{Image: "ghcr.io/example/orders", ContainerPort: 8080, Replicas: 1, Resources: applicationservice.ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"}, Service: applicationservice.ServiceSpec{Port: 80, TargetPort: 8080}, Config: map[string]string{"DATABASE_HOST": "mysql.internal", "DATABASE_NAME": "orders", "DATABASE_USER": "orders"}, Secrets: map[string]string{"DATABASE_PASSWORD": ""}, Migration: &applicationservice.MigrationSpec{Enabled: true, Command: []string{"/usr/local/bin/cylism-migrate"}, Directory: "/app/migrations", DatabasePrefix: "DATABASE", TimeoutSeconds: 30}}
	snapshot, _ := json.Marshal(spec)
	encrypted, _ := crypto.Encrypt(key, "{\"DATABASE_PASSWORD\":\"private-db-password\"}")
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "prod", Enabled: true, Spec: string(snapshot), EncryptedSecrets: encrypted}
	if err = st.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}
	client := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: app.Environment.Namespace}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}})
	client.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*batchv1.Job)
		job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
		return false, nil, nil
	})
	handler := newTestApplicationHandler(st, key, NewKubernetesAdapter(&k8s.Client{Clientset: client}))
	router := gin.New()
	router.POST("/api/applications/:id/releases/preflight", handler.PreflightRelease)
	router.POST("/api/applications/:id/deploy-webhook/secret", handler.GenerateDeployWebhookSecret)
	router.POST("/api/applications/:id/deployments", handler.DeployWebhook)
	generated := serve(router, newJSONRequest(http.MethodPost, fmt.Sprintf("/api/applications/%d/deploy-webhook/secret", app.ID), nil))
	var response struct{ Data struct{ Secret string } }
	if err = json.Unmarshal(generated.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/applications/%d/deployments", app.ID)
	for i, body := range []string{
		"{\"version\":\"1.0\"}",
		"{\"version\":\"1.0\",\"migration\":{\"target\":\"0\"}}",
		"{\"version\":\"1.0\",\"migration\":{\"target\":\"1\",\"command\":[\"sh\"]}}",
		"{\"version\":\"1.0\",\"migration\":{\"target\":\"1\",\"database\":\"other\"}}",
	} {
		res := serve(router, signedApplicationWebhookRequest(path, body, time.Now().Unix(), fmt.Sprintf("invalid-migration-%02d", i), response.Data.Secret))
		if res.Code < 400 {
			t.Fatalf("invalid migration accepted: %d %s", res.Code, res.Body.String())
		}
	}
	payload := gin.H{"template_id": template.ID, "version": "1.0", "migration": gin.H{"target": "202611220001"}}
	preflight := serve(router, newJSONRequest(http.MethodPost, fmt.Sprintf("/api/applications/%d/releases/preflight", app.ID), payload))
	if preflight.Code != http.StatusOK || strings.Contains(preflight.Body.String(), "private-db-password") {
		t.Fatalf("preflight: %d %s", preflight.Code, preflight.Body.String())
	}
	releases, _ := st.ListReleases(app.ID)
	jobs, _ := client.BatchV1().Jobs(app.Environment.Namespace).List(context.Background(), metav1.ListOptions{})
	if len(releases) != 0 || len(jobs.Items) != 0 {
		t.Fatal("preflight changed cluster or release history")
	}
	body := "{\"version\":\"1.0\",\"migration\":{\"target\":\"202611220001\"}}"
	accepted := serve(router, signedApplicationWebhookRequest(path, body, time.Now().Unix(), "valid-migration-nonce-123", response.Data.Secret))
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("callback: %d %s", accepted.Code, accepted.Body.String())
	}
	releases, _ = st.ListReleases(app.ID)
	if len(releases) != 1 || !strings.Contains(releases[0].DesiredSpec, "202611220001") || strings.Contains(releases[0].DesiredSpec, "private-db-password") {
		t.Fatalf("unsafe snapshot: %+v", releases)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		release, _ := st.GetRelease(releases[0].ID)
		if release.Status == model.ReleaseStatusFailed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("failed migration Job did not stop release")
}
