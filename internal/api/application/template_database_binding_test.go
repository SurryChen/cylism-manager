package applicationapi

import (
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
)

func TestValidateBoundDatabaseTemplate(t *testing.T) {
	current := &model.ApplicationDeploymentTemplate{Spec: `{"config":{"DATABASE_HOST":"mysql.internal","DATABASE_PORT":"3306","DATABASE_NAME":"app_db","DATABASE_USER":"app_user"},"secrets":{"DATABASE_PASSWORD":""}}`}
	valid := func() applicationservice.ReleaseSpec {
		return applicationservice.ReleaseSpec{
			Config:  map[string]string{"DATABASE_HOST": "mysql.internal", "DATABASE_PORT": "3306", "DATABASE_NAME": "app_db", "DATABASE_USER": "app_user", "LOG_LEVEL": "debug"},
			Secrets: map[string]string{"DATABASE_PASSWORD": ""},
		}
	}
	if err := validateBoundDatabaseTemplate(current, valid(), "DATABASE"); err != nil {
		t.Fatalf("unrelated template edit rejected: %v", err)
	}
	changedHost := valid()
	changedHost.Config["DATABASE_HOST"] = "other.internal"
	if err := validateBoundDatabaseTemplate(current, changedHost, "DATABASE"); err == nil || !strings.Contains(err.Error(), "DATABASE_HOST") {
		t.Fatalf("changed database host accepted: %v", err)
	}
	removedPassword := valid()
	delete(removedPassword.Secrets, "DATABASE_PASSWORD")
	if err := validateBoundDatabaseTemplate(current, removedPassword, "DATABASE"); err == nil {
		t.Fatal("removed database password accepted")
	}
	changedPassword := valid()
	changedPassword.Secrets["DATABASE_PASSWORD"] = "new-password"
	if err := validateBoundDatabaseTemplate(current, changedPassword, "DATABASE"); err == nil {
		t.Fatal("replaced database password accepted")
	}
	fileConfig := valid()
	fileConfig.ConfigFileKeys = []string{"DATABASE_HOST"}
	if err := validateBoundDatabaseTemplate(current, fileConfig, "DATABASE"); err == nil {
		t.Fatal("file-only database host accepted")
	}
	fileSecret := valid()
	fileSecret.SecretFileKeys = []string{"DATABASE_PASSWORD"}
	if err := validateBoundDatabaseTemplate(current, fileSecret, "DATABASE"); err == nil {
		t.Fatal("file-only database password accepted")
	}
	disabled := valid()
	disabled.ConfigDisabled = []string{"DATABASE_HOST"}
	if err := validateBoundDatabaseTemplate(current, disabled, "DATABASE"); err == nil {
		t.Fatal("disabled database host accepted")
	}
}
