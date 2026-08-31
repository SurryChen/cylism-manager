package registry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

func newMirrorServiceTest(t *testing.T) (*MirrorService, *store.Store) {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if db, err := s.DB().DB(); err == nil {
		db.SetMaxOpenConns(1)
	}
	return NewMirrorService(s, []byte("01234567890123456789012345678901")), s
}

func TestMirrorServiceCreateEncryptsCredentialAndRejectsWrongVerificationRegistry(t *testing.T) {
	service, _ := newMirrorServiceTest(t)
	_, err := service.Create(MirrorInput{
		Name:              "docker-hub-mirror",
		Registry:          "docker.io",
		Endpoints:         []string{"https://mirror.example.com"},
		VerificationImage: "ghcr.io/example/busybox:1.36",
	}, 9)
	if err == nil || !strings.Contains(err.Error(), "验证镜像必须属于当前 Registry") {
		t.Fatalf("expected verification registry error, got %v", err)
	}

	mirror, err := service.Create(MirrorInput{
		Name:              "docker-hub-mirror",
		Registry:          "docker.io",
		Endpoints:         []string{"https://mirror.example.com"},
		VerificationImage: "docker.io/library/busybox:1.36",
		Username:          "cylism",
		Credential:        "secret-token",
	}, 9)
	if err != nil {
		t.Fatal(err)
	}
	if mirror.CreatedBy != 9 || !mirror.CredentialConfigured || mirror.Credential != "" {
		t.Fatalf("expected a redacted persisted result, got %#v", mirror)
	}
}

func TestMirrorServiceVerifyPersistsResult(t *testing.T) {
	service, _ := newMirrorServiceTest(t)
	mirror, err := service.Create(MirrorInput{Name: "docker-hub-mirror", Registry: "docker.io", Endpoints: []string{"https://mirror.example.com"}, VerificationImage: "docker.io/library/busybox:1.36"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := service.Verify(context.Background(), mirror.ID, func(context.Context, *model.NodeRegistryMirror) error {
		return errors.New("https://mirror.example.com: 认证失败 (HTTP 401)")
	})
	if err != nil {
		t.Fatal(err)
	}
	if verified.LastVerifyStatus != "failed" || !strings.Contains(verified.LastVerifyError, "认证失败") {
		t.Fatalf("unexpected verification result: %#v", verified)
	}
}

func TestMirrorServiceApplyUsesOnlySelectedClusterNodes(t *testing.T) {
	service, s := newMirrorServiceTest(t)
	mirror, err := service.Create(MirrorInput{Name: "docker-hub-mirror", Registry: "docker.io", Endpoints: []string{"https://mirror.example.com"}, VerificationImage: "docker.io/library/busybox:1.36"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	worker := &model.Server{Name: "worker-a", Host: "10.0.0.11", ClusterRole: "worker", K8sNodeName: "worker-a"}
	if err := s.CreateServer(worker); err != nil {
		t.Fatal(err)
	}
	standalone := &model.Server{Name: "standalone", Host: "10.0.0.12"}
	if err := s.CreateServer(standalone); err != nil {
		t.Fatal(err)
	}

	started := make(chan uint, 1)
	finish := make(chan struct{})
	updated, err := service.StartApply(mirror.ID, []uint{worker.ID}, func(server *model.Server, _ []byte) (string, string) {
		started <- server.ID
		<-finish
		return "success", "配置已写入"
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.LastApplyStatus != "applying" {
		t.Fatalf("expected applying status, got %#v", updated)
	}
	select {
	case got := <-started:
		if got != worker.ID {
			t.Fatalf("expected worker %d, got %d", worker.ID, got)
		}
	case <-time.After(time.Second):
		t.Fatal("apply did not start")
	}
	if _, err := service.StartApply(mirror.ID, []uint{worker.ID}, func(*model.Server, []byte) (string, string) { return "success", "" }); !errors.Is(err, ErrMirrorApplyRunning) {
		t.Fatalf("expected in-flight error, got %v", err)
	}
	if _, err := service.StartApply(mirror.ID, []uint{standalone.ID}, func(*model.Server, []byte) (string, string) { return "success", "" }); err == nil || !strings.Contains(err.Error(), "不是可应用的集群节点") {
		t.Fatalf("expected invalid node error, got %v", err)
	}
	close(finish)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		result, getErr := service.Get(mirror.ID)
		if getErr == nil && result.LastApplyStatus == "succeeded" {
			if len(result.NodeStatuses) != 1 || result.NodeStatuses[0].ServerID != worker.ID {
				t.Fatalf("unexpected node result: %#v", result.NodeStatuses)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("apply did not complete")
}
