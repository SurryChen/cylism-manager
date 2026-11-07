package gateway

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func fixture(t *testing.T) (*Service, *model.DatabaseInstance, model.DatabaseGateway) {
	t.Helper()
	s, err := store.New(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	t.Cleanup(func() { _ = db.Close() })
	server := model.Server{Name: "private-ecs", Host: "192.0.2.1", SSHUser: "admin"}
	cluster := model.Cluster{Name: "apps", Distribution: "k3s", Status: "ready"}
	instance := &model.DatabaseInstance{Name: "rds", Mode: "external", Host: "mysql.private.internal", Port: 3306, DatabaseName: "app_db", Username: "app_user", EncryptedPassword: "encrypted", Status: "registered"}
	for _, item := range []any{&server, &cluster, instance} {
		if err := s.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	service := New(s, nil, func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	service.exec = func(context.Context, time.Duration, *model.Server, string) ([]byte, error) { return nil, nil }
	service.probe = func(_ context.Context, _ *k8s.Client, _ *model.DatabaseGateway, log func(string) error) error {
		return log("all nodes 3/3")
	}
	return service, instance, model.DatabaseGateway{ServerID: server.ID, ClusterID: cluster.ID, ListenIP: "100.80.1.1", ListenPort: 13306, AllowedSources: []string{"100.80.1.2", "10.42.0.0/16"}}
}

func TestGatewayQueuesRecoversAndDisablesWithoutLosingHistory(t *testing.T) {
	s, instance, input := fixture(t)
	// Network-only registration has no business database, user or password.
	instance.DatabaseName, instance.Username, instance.EncryptedPassword = "", "", ""
	if err := s.store.DB().Model(instance).Updates(map[string]any{"database_name": "", "username": "", "encrypted_password": ""}).Error; err != nil {
		t.Fatal(err)
	}
	executions := 0
	s.exec = func(context.Context, time.Duration, *model.Server, string) ([]byte, error) {
		executions++
		return nil, nil
	}
	config, err := s.Configure(instance, input)
	if err != nil {
		t.Fatal(err)
	}
	if executions != 0 {
		t.Fatal("configuration must only queue work")
	}
	if _, err := s.Queue(instance.ID, "disable"); err == nil {
		t.Fatal("duplicate active task accepted")
	}
	var task model.DatabaseGatewayTask
	s.store.DB().First(&task)
	// Simulate a platform process stopping after claiming a task.
	s.store.DB().Model(&task).Update("status", "running")
	s.Reconcile(context.Background())
	s.store.DB().First(&task, task.ID)
	s.store.DB().First(config, config.ID)
	if task.Status != "succeeded" || config.Status != "ready" || config.LastVerifiedAt == nil {
		t.Fatalf("recovery failed: %+v %+v", task, config)
	}
	client, _ := s.client(context.Background(), config.ClusterID)
	if _, err := client.Clientset.CoreV1().Services(namespace).Get(context.Background(), Name(config.ID), metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
	var logs []model.DatabaseGatewayTaskLog
	s.store.DB().Where("task_id = ?", task.ID).Find(&logs)
	if len(logs) < 4 || !strings.Contains(logs[0].Message, "恢复") {
		t.Fatalf("missing persistent recovery log: %+v", logs)
	}
	if _, err := s.Queue(instance.ID, "disable"); err != nil {
		t.Fatal(err)
	}
	s.Reconcile(context.Background())
	s.store.DB().First(config, config.ID)
	if config.Status != "disabled" {
		t.Fatalf("not disabled: %+v", config)
	}
	if _, err := client.Clientset.CoreV1().Services(namespace).Get(context.Background(), Name(config.ID), metav1.GetOptions{}); err == nil {
		t.Fatal("service not removed")
	}
	if _, err := s.Queue(instance.ID, "check"); err == nil {
		t.Fatal("disabled gateway accepted a connectivity check")
	}
	var count int64
	s.store.DB().Model(&model.DatabaseGatewayTask{}).Count(&count)
	if count != 2 {
		t.Fatalf("task history lost: %d", count)
	}
}

func TestGatewayFailedProbeIsDegradedAndPreservesServiceForDiagnosis(t *testing.T) {
	s, instance, input := fixture(t)
	config, err := s.Configure(instance, input)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.probe = func(context.Context, *k8s.Client, *model.DatabaseGateway, func(string) error) error {
		calls++
		return errors.New("node-a: denied by allowlist")
	}
	s.Reconcile(context.Background())
	s.Reconcile(context.Background())
	s.store.DB().First(config, config.ID)
	if config.Status != "degraded" || !strings.Contains(config.LastError, "allowlist") || calls != 1 {
		t.Fatalf("failure was lost or retried without request: %+v calls=%d", config, calls)
	}
	if config.LastVerifiedAt != nil {
		t.Fatal("failed test was marked verified")
	}
	client, _ := s.client(context.Background(), input.ClusterID)
	if _, err := client.Clientset.CoreV1().Services(namespace).Get(context.Background(), Name(config.ID), metav1.GetOptions{}); err != nil {
		t.Fatal("failed deployment should retain inspectable service")
	}
	s.probe = func(context.Context, *k8s.Client, *model.DatabaseGateway, func(string) error) error { return nil }
	if _, err := s.Queue(instance.ID, "check"); err != nil {
		t.Fatal(err)
	}
	s.Reconcile(context.Background())
	s.store.DB().First(config, config.ID)
	if config.Status != "ready" || config.LastVerifiedAt == nil || config.LastError != "" {
		t.Fatalf("retry failed: %+v", config)
	}
}

func TestGatewayShutdownKeepsRunningTaskForNextProcess(t *testing.T) {
	s, instance, input := fixture(t)
	if _, err := s.Configure(instance, input); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.exec = func(context.Context, time.Duration, *model.Server, string) ([]byte, error) {
		cancel()
		return nil, context.Canceled
	}
	s.Reconcile(ctx)
	var task model.DatabaseGatewayTask
	s.store.DB().First(&task)
	if task.Status != "running" {
		t.Fatalf("shutdown should remain resumable, got %s", task.Status)
	}
	s.exec = func(context.Context, time.Duration, *model.Server, string) ([]byte, error) { return nil, nil }
	s.Reconcile(context.Background())
	s.store.DB().First(&task, task.ID)
	if task.Status != "succeeded" {
		t.Fatalf("interrupted task did not resume: %+v", task)
	}
}

func TestGatewayHealthCheckCannotOverwriteNewTask(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			s, instance, input := fixture(t)
			config, err := s.Configure(instance, input)
			if err != nil {
				t.Fatal(err)
			}
			s.Reconcile(context.Background())
			s.store.DB().Model(config).Update("last_checked_at", time.Now().Add(-2*time.Minute))
			s.exec = func(context.Context, time.Duration, *model.Server, string) ([]byte, error) {
				if _, err := s.Queue(instance.ID, "disable"); err != nil {
					t.Fatal(err)
				}
				if failure {
					return nil, errors.New("old health check failed")
				}
				return nil, nil
			}
			s.Reconcile(context.Background())
			s.store.DB().First(config, config.ID)
			if config.Status != "disabling" || config.LastError != "" {
				t.Fatalf("health result overwrote newer task: %+v", config)
			}
		})
	}
}

func TestGatewayPeriodicHealthCanDegradeAndRecover(t *testing.T) {
	s, instance, input := fixture(t)
	config, err := s.Configure(instance, input)
	if err != nil {
		t.Fatal(err)
	}
	s.Reconcile(context.Background())
	for _, state := range []string{"degraded", "ready"} {
		s.store.DB().Model(config).Update("last_checked_at", time.Now().Add(-2*time.Minute))
		s.exec = func(_ context.Context, _ time.Duration, _ *model.Server, command string) ([]byte, error) {
			if !strings.Contains(command, "--health") || strings.Contains(command, "--check") {
				t.Fatal("periodic health must not open a database connection")
			}
			if state == "degraded" {
				return nil, errors.New("gateway unavailable")
			}
			return nil, nil
		}
		s.Reconcile(context.Background())
		s.store.DB().First(config, config.ID)
		if config.Status != state || config.LastVerifiedAt == nil {
			t.Fatalf("unexpected health status: %+v", config)
		}
	}
}

func TestGatewayConfigurationRejectsListenerCollisionsAndMutation(t *testing.T) {
	s, instance, input := fixture(t)
	config, err := s.Configure(instance, input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := model.DatabaseGatewayTask{GatewayID: config.ID, Kind: "deploy", Status: "queued"}
	if err := s.store.DB().Create(&duplicate).Error; err == nil {
		t.Fatal("database must enforce one active task per gateway")
	}
	s.Reconcile(context.Background())
	input.ListenPort++
	if _, err := s.Configure(instance, input); err == nil {
		t.Fatal("existing listener unexpectedly changed")
	}
	input.ListenPort--
	other := *instance
	other.ID = 0
	other.Name = "other"
	if err := s.store.DB().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Configure(&other, input); err == nil {
		t.Fatal("listener collision accepted")
	}
	input.AllowedSources = []string{"100.80.1.3"}
	if _, err := s.Configure(instance, input); err != nil {
		t.Fatalf("allowlist edit rejected: %v", err)
	}
}

func TestGatewayRejectsUnsafeAddressesAndShellInputs(t *testing.T) {
	instance := &model.DatabaseInstance{Mode: "external", Host: "rds.internal", Port: 3306}
	base := model.DatabaseGateway{ClusterID: 1, ServerID: 1, ListenIP: "100.80.0.1", ListenPort: 13306, AllowedSources: []string{"100.80.0.2/32"}}
	for _, test := range []struct {
		name   string
		change func(*model.DatabaseInstance, *model.DatabaseGateway)
	}{
		{"public-listener", func(_ *model.DatabaseInstance, c *model.DatabaseGateway) { c.ListenIP = "0.0.0.0" }},
		{"public-target", func(i *model.DatabaseInstance, _ *model.DatabaseGateway) { i.Host = "8.8.8.8" }},
		{"loopback-target", func(i *model.DatabaseInstance, _ *model.DatabaseGateway) { i.Host = "127.0.0.1" }},
		{"metadata-target", func(i *model.DatabaseInstance, _ *model.DatabaseGateway) { i.Host = "169.254.169.254" }},
		{"command-host", func(i *model.DatabaseInstance, _ *model.DatabaseGateway) { i.Host = "mysql;touch /tmp/unsafe" }},
		{"all-sources", func(_ *model.DatabaseInstance, c *model.DatabaseGateway) { c.AllowedSources = []string{"0.0.0.0/0"} }},
		{"cross-private-boundary", func(_ *model.DatabaseInstance, c *model.DatabaseGateway) { c.AllowedSources = []string{"10.0.0.0/7"} }},
		{"public-sources", func(_ *model.DatabaseInstance, c *model.DatabaseGateway) { c.AllowedSources = []string{"100.0.0.0/8"} }},
		{"bad-image", func(_ *model.DatabaseInstance, c *model.DatabaseGateway) { c.ProbeImage = "python image" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			item, config := *instance, base
			config.AllowedSources = append([]string(nil), base.AllowedSources...)
			test.change(&item, &config)
			if Validate(&item, &config) == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
	if err := Validate(instance, &base); err != nil {
		t.Fatal(err)
	}
	if base.ProbeImage != DefaultProbeImage {
		t.Fatal("missing default probe image")
	}
	command := DeployCommand(instance, &base)
	if strings.Contains(command, "password") || !strings.Contains(command, "--check") {
		t.Fatal("deployment command should contain only target configuration and a preflight")
	}
}

func TestGatewayCommandsHaveValidShellSyntax(t *testing.T) {
	instance := &model.DatabaseInstance{Mode: "external", Host: "rds.internal", Port: 3306}
	config := &model.DatabaseGateway{ID: 1, ListenIP: "100.80.1.1", ListenPort: 13306, AllowedSources: []string{"100.80.1.2/32"}}
	for _, source := range []string{DeployCommand(instance, config), DisableCommand(config.ID), HealthCommand(config)} {
		command := exec.Command("sh", "-n")
		command.Stdin = strings.NewReader(source)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("invalid shell: %s %v", output, err)
		}
	}
}

func TestGatewayForwarderFailureRemainsError(t *testing.T) {
	s, instance, input := fixture(t)
	config, err := s.Configure(instance, input)
	if err != nil {
		t.Fatal(err)
	}
	s.exec = func(context.Context, time.Duration, *model.Server, string) ([]byte, error) {
		return nil, errors.New("systemd failed")
	}
	s.Reconcile(context.Background())
	s.store.DB().First(config, config.ID)
	if config.Status != "error" || config.LastVerifiedAt != nil {
		t.Fatalf("real deployment failure misclassified: %+v", config)
	}
}
