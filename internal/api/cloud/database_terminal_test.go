package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestDatabaseClientJobAndCommandNeverContainPassword(t *testing.T) {
	key := bytes.Repeat([]byte("k"), 32)
	encrypted, _ := security.Encrypt(key, "secret-terminal-password")
	item := &model.DatabaseInstance{Mode: "external", AdminUsername: "dts", EncryptedAdmin: encrypted}
	username, password, err := databaseTerminalCredentials(item, "admin", key)
	if err != nil || username != "dts" || string(password) != "secret-terminal-password" {
		t.Fatalf("credential selection failed: %v", err)
	}
	session := &databaseTerminalSession{ID: "abc", jobName: "test", NodeName: "node", host: "rds.internal", port: 3306, Username: "name'; touch /tmp/bad #"}
	job := databaseClientJob(session)
	raw, _ := json.Marshal(job)
	command := databaseClientCommand(session)
	if bytes.Contains(raw, password) || strings.Contains(command, string(password)) || len(job.Spec.Template.Spec.Containers[0].Env) != 0 {
		t.Fatal("password leaked into Kubernetes spec or command")
	}
	if !strings.Contains(command, "--ssl-mode=REQUIRED") || !strings.Contains(command, "MYSQL_HISTFILE=/dev/null") || !strings.Contains(command, "'\"'\"'") {
		t.Fatalf("unsafe terminal command: %s", command)
	}
	if *job.Spec.ActiveDeadlineSeconds != 1800 || *job.Spec.TTLSecondsAfterFinished != 300 || *job.Spec.Template.Spec.AutomountServiceAccountToken || !*job.Spec.Template.Spec.Containers[0].SecurityContext.ReadOnlyRootFilesystem {
		t.Fatal("client session lacks cleanup or isolation")
	}
	for _, bad := range []string{"", "root"} {
		if _, _, err := databaseTerminalCredentials(item, bad, key); err == nil {
			t.Fatal("invalid account accepted")
		}
	}
}
func TestDatabaseSessionIsBoundToOwnerInstanceAndExpiration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDatabaseHandler(nil, nil, nil)
	session := &databaseTerminalSession{ID: "session", InstanceID: 2, owner: 7, ExpiresAt: time.Now().Add(time.Minute)}
	h.databaseSessions.Store(session.ID, session)
	for _, test := range []struct {
		instance string
		owner    uint
		expired  bool
		accepted bool
	}{{"2", 7, false, true}, {"3", 7, false, false}, {"2", 8, false, false}, {"2", 0, false, false}, {"2", 7, true, false}} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Params = gin.Params{{Key: "id", Value: test.instance}, {Key: "sessionID", Value: session.ID}}
		c.Set("user_id", test.owner)
		session.ExpiresAt = time.Now().Add(time.Minute)
		if test.expired {
			session.ExpiresAt = time.Now().Add(-time.Minute)
		}
		_, ok := h.getDatabaseSession(c)
		if ok != test.accepted {
			t.Fatalf("invalid session authorization: %+v", test)
		}
	}
}

func TestDatabaseSessionCreationNeverReturnsCredentialsAndCleansJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	defer db.Close()
	key := bytes.Repeat([]byte("k"), 32)
	encrypted, _ := security.Encrypt(key, "session-secret-password")
	item := model.DatabaseInstance{Name: "rds", Mode: "external", ConnectionMode: "gateway", Host: "rds.internal", Port: 3306, Status: "registered", AdminUsername: "dts", EncryptedAdmin: encrypted}
	if err = s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	// A diagnostic failure on an unrelated node must not block an explicit
	// terminal attempt through the selected node's existing gateway route.
	gw := model.DatabaseGateway{InstanceID: item.ID, ClusterID: 1, ServerID: 1, ListenIP: "100.64.1.2", ListenPort: 13306, Status: "error"}
	if err = s.DB().Create(&gw).Error; err != nil {
		t.Fatal(err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}})}
	h := NewDatabaseHandler(s, key, func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Next() })
	router.POST("/databases/:id/sessions", h.CreateDatabaseSession)
	router.DELETE("/databases/:id/sessions/:sessionID", h.DeleteDatabaseSession)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("POST", fmt.Sprintf("/databases/%d/sessions", item.ID), strings.NewReader(`{"cluster_id":1,"node_name":"worker","account":"admin"}`)))
	if recorder.Code != 200 || strings.Contains(recorder.Body.String(), "session-secret-password") || strings.Contains(recorder.Body.String(), encrypted) {
		t.Fatalf("session creation failed or leaked credentials: %s", recorder.Body)
	}
	var response struct {
		Data struct {
			ID string `json:"id"`
		}
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Data.ID == "" {
		t.Fatalf("missing session ID: %s", recorder.Body)
	}
	stored, _ := h.databaseSessions.Load(response.Data.ID)
	if stored.(*databaseTerminalSession).host != gateway.ServiceHost(gw.ID) {
		t.Fatal("terminal bypassed the configured private gateway")
	}
	jobs, _ := client.Clientset.BatchV1().Jobs(database.Namespace).List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Fatal("client job not created")
	}
	raw, _ := json.Marshal(jobs)
	if bytes.Contains(raw, []byte("session-secret-password")) {
		t.Fatal("job contains password")
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("DELETE", fmt.Sprintf("/databases/%d/sessions/%s", item.ID, response.Data.ID), nil))
	if recorder.Code != 200 {
		t.Fatalf("session cleanup failed: %s", recorder.Body)
	}
	jobs, _ = client.Clientset.BatchV1().Jobs(database.Namespace).List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Fatal("session job not removed")
	}
	if err = s.DB().Model(&gw).Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("POST", fmt.Sprintf("/databases/%d/sessions", item.ID), strings.NewReader(`{"cluster_id":1,"node_name":"worker","account":"admin"}`)))
	if recorder.Code == 200 {
		t.Fatal("a disabled gateway must not allow new sessions")
	}
	jobs, _ = client.Clientset.BatchV1().Jobs(database.Namespace).List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Fatal("created a client through a disabled gateway")
	}
}
