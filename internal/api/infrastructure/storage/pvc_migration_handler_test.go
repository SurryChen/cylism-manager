package storage

import (
	"context"
	"testing"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type fakePVCWorkloads struct {
	deploymentCalls int
	deployment      *appsv1.Deployment
}

func (f *fakePVCWorkloads) NamespaceExists(context.Context, string) error { return nil }
func (f *fakePVCWorkloads) ListDeployments(context.Context, string) ([]appsv1.Deployment, error) {
	return nil, nil
}
func (f *fakePVCWorkloads) ListStatefulSets(context.Context, string) ([]appsv1.StatefulSet, error) {
	return nil, nil
}
func (f *fakePVCWorkloads) GetDeployment(ctx context.Context, _, _ string) (*appsv1.Deployment, error) {
	f.deploymentCalls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.deployment, nil
}

type fakePVCMigration struct{ listPodCalls int }

func (f *fakePVCMigration) GetNodeInfo(context.Context, string) (*k8sclient.NodeInfo, error) {
	return nil, nil
}
func (f *fakePVCMigration) CreatePVCBindingPod(context.Context, string, string, string, string, string) (*corev1.Pod, error) {
	return nil, nil
}
func (f *fakePVCMigration) DeletePVCBindingPod(context.Context, string, string) error { return nil }
func (f *fakePVCMigration) WaitForManagedPVCBound(context.Context, string, string, uint) (*k8sclient.PersistentVolumeClaimInfo, error) {
	return nil, nil
}
func (f *fakePVCMigration) ReplaceDeploymentPVCNode(context.Context, string, string, string, string, string) (int32, error) {
	return 0, nil
}
func (f *fakePVCMigration) DeploymentUsingPVC(context.Context, string, string) ([]appsv1.Deployment, error) {
	return nil, nil
}
func (f *fakePVCMigration) ScaleDeployment(context.Context, string, string, int32) error { return nil }
func (f *fakePVCMigration) ListDeploymentPods(ctx context.Context, _, _ string) ([]k8sclient.PodRef, error) {
	f.listPodCalls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []k8sclient.PodRef{{Name: "still-running"}}, nil
}

func TestPVCWaitHelperDelegatesWorkloadReadsWithCancellableContext(t *testing.T) {
	workloads := &fakePVCWorkloads{deployment: &appsv1.Deployment{Status: appsv1.DeploymentStatus{AvailableReplicas: 1}}}
	migration := &fakePVCMigration{}
	h := &StorageHandler{workloads: workloads, migration: migration}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.waitForStorageDeploymentPods(ctx, "ns", "app", true, time.Minute); err == nil {
		t.Fatal("expected canceled context to stop workload wait")
	}
	if workloads.deploymentCalls != 1 {
		t.Fatalf("deployment calls=%d, want 1", workloads.deploymentCalls)
	}
}

func TestPVCWaitHelperUsesMigrationReaderForStoppedPods(t *testing.T) {
	workloads := &fakePVCWorkloads{deployment: &appsv1.Deployment{}}
	migration := &fakePVCMigration{}
	h := &StorageHandler{workloads: workloads, migration: migration}
	if err := h.waitForStorageDeploymentPods(context.Background(), "ns", "app", false, 20*time.Millisecond); err == nil {
		t.Fatal("expected timeout while fake pod remains")
	}
	if migration.listPodCalls == 0 {
		t.Fatal("expected ListDeploymentPods delegation")
	}
}
