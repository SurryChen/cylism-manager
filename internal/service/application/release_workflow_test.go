package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	crypto "github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/store"
)

func TestReleaseWorkflowCreatesReleaseFromTemplateWithRuntimeCredentials(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createTestApplication(t, st)
	key := []byte("01234567890123456789012345678901")
	credential, err := crypto.Encrypt(key, "registry-password")
	if err != nil {
		t.Fatal(err)
	}
	registry := &model.ImageRegistry{
		Name: "private", Endpoint: "registry.example.com", AuthType: "basic", Username: "deploy", Credential: credential, Enabled: true,
	}
	if err := st.CreateImageRegistry(registry, []uint{app.ProjectID}); err != nil {
		t.Fatal(err)
	}
	spec := validTestReleaseSpec()
	spec.Image = "order-api"
	spec.RegistryID = registry.ID
	spec.Secrets = map[string]string{"API_TOKEN": ""}
	templateSpec, err := json.Marshal(SanitizeReleaseSpec(spec))
	if err != nil {
		t.Fatal(err)
	}
	encryptedSecrets, err := crypto.Encrypt(key, `{"API_TOKEN":"template-secret"}`)
	if err != nil {
		t.Fatal(err)
	}
	template := &model.ApplicationDeploymentTemplate{
		ApplicationID: app.ID, Name: "default", Enabled: true, Spec: string(templateSpec), EncryptedSecrets: encryptedSecrets,
	}
	if err := st.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}

	workflow := NewReleaseWorkflow(st, key, &fakeApplier{})
	prepared, err := workflow.CreateFromTemplate(context.Background(), app, template, "1.2.3", 9)
	if err != nil {
		t.Fatalf("CreateFromTemplate: %v", err)
	}
	if prepared.Release.Image != "registry.example.com/order-api:1.2.3" || prepared.Release.TemplateID == nil || *prepared.Release.TemplateID != template.ID {
		t.Fatalf("unexpected release: %#v", prepared.Release)
	}
	if prepared.Spec.RegistryCredential != "registry-password" || prepared.Spec.Secrets["API_TOKEN"] != "template-secret" {
		t.Fatalf("runtime credentials were not resolved: %#v", prepared.Spec)
	}
	if strings.Contains(prepared.Release.DesiredSpec, "template-secret") || strings.Contains(prepared.Release.DesiredSpec, "registry-password") {
		t.Fatalf("release snapshot leaked credentials: %s", prepared.Release.DesiredSpec)
	}
}

func TestReleaseWorkflowUsesNodeAppliedMirrorForImageVerification(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	server := &model.Server{Name: "worker-a", Host: "10.0.0.11", ClusterRole: "worker", K8sNodeName: "worker-a"}
	if err := st.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	mirror := &model.NodeRegistryMirror{Name: "docker-hub", Registry: "docker.io", Endpoints: `["https://docker.1panel.live"]`, Enabled: true}
	if err := st.CreateNodeRegistryMirror(mirror); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: mirror.ID, ServerID: server.ID, Status: "success"}); err != nil {
		t.Fatal(err)
	}

	workflow := NewReleaseWorkflow(st, nil, &fakeApplier{})
	spec := ReleaseSpec{Image: "zenika/alpine-chrome:124", NodeName: "worker-a"}
	if err := workflow.PrepareRuntimeSpec(nil, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.ImageVerificationEndpoint != "https://docker.1panel.live" {
		t.Fatalf("expected applied node mirror endpoint, got %+v", spec)
	}
}
