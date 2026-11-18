package applicationapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/cylism/cylism-manager/internal/service/auth"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestIntegrationOperationsEnforceScopeAndPodOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err = app.SetCapabilities([]string{"vless"}); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateApplication(app); err != nil {
		t.Fatal(err)
	}
	clientset := k8sfake.NewSimpleClientset()
	for _, name := range []string{"managed", "other"} {
		owner := app.Name
		if name == "other" {
			owner = "unrelated"
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: app.Environment.Namespace, Labels: map[string]string{applicationservice.ManagedByLabel: applicationservice.ManagedByValue, applicationservice.ApplicationNameLabel: owner}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}}
		if _, err = clientset.CoreV1().Pods(app.Environment.Namespace).Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	release := &model.Release{ApplicationID: app.ID, Sequence: 1, Version: "1.13.18", Status: model.ReleaseStatusSucceeded, DesiredSpec: "private-template-content"}
	if err = s.CreateRelease(release); err != nil {
		t.Fatal(err)
	}
	h := newTestApplicationHandler(s, []byte("01234567890123456789012345678901"), NewKubernetesAdapter(&k8sclient.Client{Clientset: clientset}))
	claims := &auth.DelegationClaims{ProjectID: app.ProjectID, EnvironmentIDs: []uint{app.EnvironmentID}, ApplicationIDs: []uint{app.ID}, Capability: "vless", Actions: []string{"application:read", "application:logs"}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("delegation", claims); c.Next() })
	r.GET("/api/integrations/applications/:id/pods", h.IntegrationListApplicationPods)
	r.GET("/api/integrations/applications/:id/pods/:pod/logs", h.IntegrationGetApplicationPodLogs)
	r.GET("/api/integrations/applications/:id/releases", h.IntegrationListApplicationReleases)
	base := fmt.Sprintf("/api/integrations/applications/%d", app.ID)
	pods := serve(r, newJSONRequest(http.MethodGet, base+"/pods", nil))
	if pods.Code != 200 || !strings.Contains(pods.Body.String(), `"name":"managed"`) || strings.Contains(pods.Body.String(), `"name":"other"`) {
		t.Fatalf("pod scope: %d %s", pods.Code, pods.Body.String())
	}
	releases := serve(r, newJSONRequest(http.MethodGet, base+"/releases", nil))
	if releases.Code != 200 || !strings.Contains(releases.Body.String(), "1.13.18") || strings.Contains(releases.Body.String(), "private-template-content") {
		t.Fatalf("unsafe release summary: %d %s", releases.Code, releases.Body.String())
	}
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/pods/other/logs?container=main", 404},
		{"/pods/managed/logs?container=other", 400},
		{"/pods/managed/logs?container=main&tail=501", 400},
		{"/releases?limit=51", 400},
	} {
		res := serve(r, newJSONRequest(http.MethodGet, base+test.path, nil))
		if res.Code != test.status {
			t.Fatalf("%s: %d %s", test.path, res.Code, res.Body.String())
		}
	}
	claims.Actions = []string{"application:read"}
	denied := serve(r, newJSONRequest(http.MethodGet, base+"/pods/managed/logs?container=main", nil))
	if denied.Code != 403 {
		t.Fatalf("logs exposed without action: %d", denied.Code)
	}
	claims.ApplicationIDs = []uint{app.ID + 1}
	for _, path := range []string{"/pods", "/releases"} {
		res := serve(r, newJSONRequest(http.MethodGet, base+path, nil))
		if res.Code != 403 {
			t.Fatalf("cross-app operation accepted: %s %d", path, res.Code)
		}
	}
}
