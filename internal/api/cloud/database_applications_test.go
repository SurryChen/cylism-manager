package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/database"
	"github.com/cylism/cylism-manager/internal/service/database/gateway"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListApplicationsDiscoversTemplatesWithoutChangingBindings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	t.Cleanup(func() { _ = db.Close() })
	key := bytes.Repeat([]byte("c"), 32)
	item := model.DatabaseInstance{Name: "rds", Mode: "external", Host: "rds.internal", Port: 3306}
	env := model.Environment{Name: "prod", Namespace: "apps", ClusterID: 1}
	for _, value := range []any{&item, &env} {
		if err := s.DB().Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	app := model.Application{Name: "account", EnvironmentID: env.ID}
	s.DB().Create(&app)
	secrets, _ := security.Encrypt(key, `{"MYSQL_DSN":"account:super-secret@tcp(rds.internal:3306)/cylism_account"}`)
	template := model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "prod", Revision: 3, Spec: `{"secrets":{"MYSQL_DSN":""}}`, EncryptedSecrets: secrets}
	s.DB().Create(&template)
	fileSecrets, _ := security.Encrypt(key, `{"config.yaml":"mysql:\n  host: rds.internal\n  database: cylism_bill\n  user: bill\n  password: never-return"}`)
	file := model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "file", Revision: 2, Spec: `{"secrets":{"config.yaml":""},"secret_file_keys":["config.yaml"],"file_mounts":[{"source_type":"application_secret","key":"config.yaml","mount_path":"/app/config.yaml"}]}`, EncryptedSecrets: fileSecrets}
	s.DB().Create(&file)
	disabled := model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "disabled", Spec: `{"secrets":{"MYSQL_DSN":""},"secrets_disabled":["MYSQL_DSN"]}`, EncryptedSecrets: secrets}
	s.DB().Create(&disabled)
	binding := model.DatabaseBinding{InstanceID: item.ID, TemplateID: template.ID, ApplicationID: app.ID, Prefix: "MYSQL", DatabaseName: "cylism_account", Username: "account"}
	s.DB().Create(&binding)
	h := NewDatabaseHandler(s, key, nil)
	r := gin.New()
	r.GET("/databases/:id/applications", h.ListApplications)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/databases/1/applications", nil))
	var response struct {
		Data struct {
			Items    []databaseApplicationView `json:"items"`
			Warnings []string                  `json:"warnings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(response.Data.Items) != 2 || len(response.Data.Warnings) != 0 {
		t.Fatalf("missing application connections: %s", w.Body.String())
	}
	if response.Data.Items[0].BindingID != binding.ID || len(response.Data.Items[0].Sources) != 2 || response.Data.Items[1].DatabaseName != "cylism_bill" {
		t.Fatalf("incorrect merged connections: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "super-secret") || strings.Contains(w.Body.String(), "never-return") || strings.Contains(w.Body.String(), "tcp(") {
		t.Fatal("connection discovery leaked secret content")
	}
	var count int64
	s.DB().Model(&model.DatabaseBinding{}).Count(&count)
	s.DB().First(&template, template.ID)
	if count != 1 || template.Revision != 3 {
		t.Fatal("read-only discovery modified templates or bindings")
	}
}

func TestDatabaseEndpointMatchingHonorsPortsAndClusterScope(t *testing.T) {
	item := &model.DatabaseInstance{Host: "rds.internal", Port: 3306, Mode: "external"}
	gw := []model.DatabaseGateway{{ID: 3, ClusterID: 1, ListenIP: "100.80.1.1", ListenPort: 13306}}
	for _, test := range []struct {
		host    string
		port    int
		cluster uint
		want    bool
	}{
		{"RDS.INTERNAL.", 3306, 1, true}, {"rds.internal.evil", 3306, 1, false}, {"rds.internal", 3307, 1, false},
		{gateway.ServiceHost(3), 3306, 1, true}, {gateway.ServiceHost(3), 3306, 2, false},
		{"cylism-db-gateway-3.cylism-databases", 3306, 1, true}, {"100.80.1.1", 13306, 1, true},
	} {
		if got := matchesDatabaseEndpoint(item, gw, test.cluster, database.ConnectionHint{Host: test.host, Port: test.port}); got != test.want {
			t.Errorf("%+v: got %v", test, got)
		}
	}
}

func TestApplicationDiscoveryReadsOnlyReferencedSecretKeysAndCachesReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	t.Cleanup(func() { _ = db.Close() })
	item := model.DatabaseInstance{Name: "rds", Mode: "external", Host: "rds.internal", Port: 3306}
	env := model.Environment{Name: "prod", Namespace: "apps", ClusterID: 1}
	s.DB().Create(&item)
	s.DB().Create(&env)
	app := model.Application{Name: "account", EnvironmentID: env.ID}
	s.DB().Create(&app)
	spec := `{"file_mounts":[{"source_type":"secret","source_name":"existing","key":"config.yaml","mount_path":"/app/config.yaml"}]}`
	for _, name := range []string{"production", "preview"} {
		s.DB().Create(&model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: name, Spec: spec})
	}
	clientset := fake.NewSimpleClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: "apps"}, Data: map[string][]byte{
		"config.yaml": []byte("mysql:\n  host: rds.internal\n  database: account\n  user: account\n  password: never-return"),
		"unused.env":  []byte("MYSQL_DSN=unused:secret@tcp(rds.internal:3306)/unused"),
	}})
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("c"), 32), func(_ context.Context, cluster uint) (*k8s.Client, error) {
		if cluster != 1 {
			t.Fatalf("unexpected cluster %d", cluster)
		}
		return &k8s.Client{Clientset: clientset}, nil
	})
	r := gin.New()
	r.GET("/databases/:id/applications", h.ListApplications)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/databases/1/applications", nil))
	var response struct {
		Data struct {
			Items    []databaseApplicationView `json:"items"`
			Warnings []string                  `json:"warnings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(response.Data.Items) != 2 || len(response.Data.Warnings) != 0 || len(clientset.Actions()) != 1 {
		t.Fatalf("unexpected scan: %s, reads=%d", w.Body.String(), len(clientset.Actions()))
	}
	if strings.Contains(w.Body.String(), "never-return") || strings.Contains(w.Body.String(), "unused") {
		t.Fatal("read unreferenced key or returned password")
	}
}
