package maintenance

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestImagePruneCommandsUseNonInteractiveSudo(t *testing.T) {
	command := ContainerImagePruneCommand()
	for _, expected := range []string{"sudo -n /usr/local/bin/crictl images", "sudo -n /usr/local/bin/crictl rmi --prune", "sudo -n /var/lib/rancher/k3s/bin/crictl images", "sudo -n /var/lib/rancher/k3s/bin/crictl rmi --prune", "sudo -n /usr/local/bin/k3s crictl images", "sudo -n /usr/local/bin/k3s crictl rmi --prune"} {
		if !strings.Contains(command, expected) {
			t.Fatalf("missing %q", expected)
		}
	}
	docker := DockerImagePruneCommand()
	for _, expected := range []string{"sudo -n /usr/bin/docker system df", "sudo -n /usr/bin/docker image prune -af", "sudo -n /usr/local/bin/docker system df", "sudo -n /usr/local/bin/docker image prune -af"} {
		if !strings.Contains(docker, expected) {
			t.Fatalf("missing %q", expected)
		}
	}
	if strings.Contains(docker, "volume") || strings.Contains(docker, "container prune") {
		t.Fatalf("unsafe docker command: %s", docker)
	}
}

func TestCleanupServiceFailureRedactsOutput(t *testing.T) {
	ssh := SSHExecutorFunc(func(_ context.Context, _ time.Duration, _ *model.Server, _ string) ([]byte, error) {
		return []byte("token=secret-value permission denied"), fmt.Errorf("exit status 1")
	})
	service := NewCleanupService(ssh)
	output, err := service.Execute(context.Background(), &model.Server{Host: "node"}, "journal-vacuum")
	if err == nil || strings.Contains(output, "secret-value") {
		t.Fatalf("unexpected output=%q err=%v", output, err)
	}
}
