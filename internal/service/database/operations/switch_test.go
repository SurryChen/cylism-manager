package operations

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
)

func TestSwitchConnectionsCommitsAtomicallyAndSurvivesRestart(t *testing.T) {
	s, _ := testService(t)
	project := model.Project{Name: "project"}
	mustCreate(t, s, &project)
	environment := model.Environment{ProjectID: project.ID, Name: "test", ClusterID: 1, Namespace: "apps"}
	mustCreate(t, s, &environment)
	app := model.Application{ProjectID: project.ID, EnvironmentID: environment.ID, Name: "app"}
	mustCreate(t, s, &app)
	target := model.DatabaseInstance{Name: "rds", Mode: "external", Host: "rds.internal", Port: 3306}
	mustCreate(t, s, &target)
	template := model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Revision: 1, Spec: `{"image":"app:1","config":{"KEEP":"value"}}`}
	mustCreate(t, s, &template)
	binding := model.DatabaseBinding{InstanceID: target.ID, ApplicationID: app.ID, TemplateID: template.ID, Prefix: "MYSQL"}
	mustCreate(t, s, &binding)
	input := jobInput{Connections: []SwitchConnection{{BindingID: binding.ID, Database: "app_db", Username: "app_user", Password: "private-business-password", Revision: 1}}}
	raw, _ := json.Marshal(input)
	encrypted, _ := security.Encrypt(s.key, string(raw))
	task := model.DatabaseOperation{Kind: "switch", Target: Reference(target.ID), Spec: "{}", Status: "running", EncryptedInput: encrypted}
	mustCreate(t, s, &task)
	// A revision increment on the same target is not evidence this task committed.
	s.store.DB().Model(&template).Update("revision", 2)
	if err := s.switchConnections(&task); err == nil {
		t.Fatal("mistook unrelated edit for successful switch")
	}
	s.store.DB().Model(&template).Update("revision", 1)
	if err := s.switchConnections(&task); err != nil {
		t.Fatal(err)
	}
	s.store.DB().First(&template, template.ID)
	if template.Revision != 2 || strings.Contains(template.Spec, "private-business-password") || !strings.Contains(template.Spec, "KEEP") || !strings.Contains(template.Spec, "rds.internal") {
		t.Fatal("connection update lost unrelated config or leaked password")
	}
	plain, err := security.Decrypt(s.key, template.EncryptedSecrets)
	if err != nil || !strings.Contains(plain, "private-business-password") {
		t.Fatal("missing encrypted template password")
	}
	restarted := New(s.store, s.key, s.client)
	if err := restarted.switchConnections(&task); err != nil {
		t.Fatal(err)
	}
	s.store.DB().First(&template, template.ID)
	if template.Revision != 2 {
		t.Fatal("replayed successful switch after restart")
	}
}

func TestSwitchRollbackOnLaterTemplateConflict(t *testing.T) {
	s, _ := testService(t)
	project := model.Project{Name: "project"}
	mustCreate(t, s, &project)
	env := model.Environment{ProjectID: project.ID, Name: "test", ClusterID: 1}
	mustCreate(t, s, &env)
	app := model.Application{ProjectID: project.ID, EnvironmentID: env.ID, Name: "app"}
	mustCreate(t, s, &app)
	target := model.DatabaseInstance{Name: "rds", Mode: "external", Host: "rds.internal", Port: 3306}
	mustCreate(t, s, &target)
	input := jobInput{}
	for _, name := range []string{"first", "second"} {
		template := model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: name, Revision: 1, Spec: `{"image":"app:1"}`}
		mustCreate(t, s, &template)
		binding := model.DatabaseBinding{InstanceID: target.ID, ApplicationID: app.ID, TemplateID: template.ID, Prefix: "MYSQL"}
		mustCreate(t, s, &binding)
		revision := uint(1)
		if name == "second" {
			revision = 99
		}
		input.Connections = append(input.Connections, SwitchConnection{BindingID: binding.ID, Database: "app_db", Username: "app_user", Password: "password", Revision: revision})
	}
	raw, _ := json.Marshal(input)
	encrypted, _ := security.Encrypt(s.key, string(raw))
	task := model.DatabaseOperation{Kind: "switch", Target: Reference(target.ID), Status: "running", Spec: "{}", EncryptedInput: encrypted}
	mustCreate(t, s, &task)
	if err := s.switchConnections(&task); err == nil {
		t.Fatal("conflicting template accepted")
	}
	var template model.ApplicationDeploymentTemplate
	s.store.DB().Where("name = ?", "first").First(&template)
	if template.Revision != 1 || template.EncryptedSecrets != "" {
		t.Fatal("partial switch survived rollback")
	}
	if readTask(t, s, task.ID).ConnectionsApplied {
		t.Fatal("rolled back switch marked committed")
	}
}
