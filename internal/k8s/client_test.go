package k8s

import (
	"context"
	"testing"
)

func TestNewClient_WithFakeConfig(t *testing.T) {
	// 从已有节点测试中确认 fake clientset 模式可用
	client, err := newClientFromRestConfig(nil)
	if err != nil {
		t.Fatalf("newClientFromRestConfig failed: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Clientset == nil {
		t.Fatal("expected non-nil clientset")
	}
	if client.DynamicClient == nil {
		t.Fatal("expected non-nil dynamic client")
	}
}

func TestCheckCRD_NotFound(t *testing.T) {
	// 无 K8s 集群时 skip
	t.Skip("需要 K8s 集群")
}

func TestClientContext(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	ctx := context.WithValue(context.Background(), "test", "value")
	view := client.withContext(ctx)
	if view == nil || view.ctx != ctx {
		t.Fatal("expected context-bound client view")
	}
}
