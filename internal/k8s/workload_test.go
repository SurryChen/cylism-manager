package k8s

import (
	"testing"
)

// MockWorkloadClient 使用 fake clientset 验证工作量方法签名和结构
type MockWorkloadClient struct {
	clientset interface{}
}

func TestNewWorkloadMethods_Exist(t *testing.T) {
	// 验证 workload.go 中的核心方法签名可用
	// 实际 K8s API 交互依赖真实集群，此处验证编译期方法存在
	client, _ := newClientFromRestConfig(nil)
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	// 验证关键方法可调用（nil clientset 会在调用时 panic，仅验证编译通过）
	_ = client.ListDeployments
	_ = client.GetDeployment
	_ = client.ListDeploymentPods
	_ = client.ListDeploymentRevisions
	_ = client.ScaleDeployment
	_ = client.UpdateDeploymentImage
	_ = client.RollbackDeployment
	_ = client.ListStatefulSets
	_ = client.GetStatefulSet
	_ = client.ScaleStatefulSet
	_ = client.ListDaemonSets
	_ = client.GetDaemonSet
}

func TestDeploymentInfo_FieldsComplete(t *testing.T) {
	// 验证 DeploymentInfo 结构体字段
	d := DeploymentInfo{
		Name:      "test-deploy",
		Namespace: "default",
		Replicas:  3,
		Ready:     2,
		Images:    []string{"nginx:1.25"},
		CPU:       "100m",
		Memory:    "128Mi",
		Age:       "5d",
	}
	if d.Name != "test-deploy" {
		t.Errorf("unexpected name: %s", d.Name)
	}
	if len(d.Images) != 1 {
		t.Errorf("expected 1 image, got %d", len(d.Images))
	}
}

func TestStatefulSetInfo_FieldsComplete(t *testing.T) {
	s := StatefulSetInfo{
		Name:      "test-sts",
		Namespace: "default",
		Replicas:  2,
		Ready:     2,
		Images:    []string{"redis:7"},
		Age:       "3d",
	}
	if s.Name != "test-sts" {
		t.Errorf("unexpected name: %s", s.Name)
	}
}

func TestDaemonSetInfo_FieldsComplete(t *testing.T) {
	d := DaemonSetInfo{
		Name:      "test-ds",
		Namespace: "kube-system",
		Desired:   3,
		Ready:     3,
		Images:    []string{"traefik:2.10"},
		NodeSelector: map[string]string{"kubernetes.io/os": "linux"},
		Age:           "7d",
	}
	if d.Name != "test-ds" {
		t.Errorf("unexpected name: %s", d.Name)
	}
}

func TestPodRef_FieldsComplete(t *testing.T) {
	p := PodRef{
		Name:      "test-pod-abc123",
		Namespace: "default",
		Status:    "Running",
		Node:      "node-1",
		IP:        "10.42.0.15",
		Restarts:  0,
		Age:       "2d",
	}
	if p.IP != "10.42.0.15" {
		t.Errorf("unexpected IP: %s", p.IP)
	}
}

func TestRevisionInfo_FieldsComplete(t *testing.T) {
	r := RevisionInfo{
		Revision: 3,
		Image:    "nginx:1.25",
		Replicas: 3,
		Age:      "1d",
		Cause:    "update image",
	}
	if r.Revision != 3 {
		t.Errorf("unexpected revision: %d", r.Revision)
	}
}
