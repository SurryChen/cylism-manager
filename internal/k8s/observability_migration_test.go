package k8s

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func migrationClient(t *testing.T, component string) *Client {
	t.Helper()
	client := &Client{Clientset: k8sfake.NewSimpleClientset(readyMigrationNode("a"), readyMigrationNode("b"))}
	var err error
	if component == InfrastructureLoki {
		_, err = client.InstallLoggingContext(t.Context(), LoggingConfig{NodeName: "a", Storage: "5Gi", RetentionDays: 14})
	} else {
		_, err = client.InstallVictoriaMetricsContext(t.Context(), VictoriaMetricsConfig{NodeName: "a", Storage: "5Gi", RetentionDays: 14})
	}
	if err != nil {
		t.Fatal(err)
	}
	_, volume, _ := componentWorkload(component)
	pvc := lokiPVCName
	if volume == "storage" {
		pvc = victoriaMetricsPVCName
	}
	bindMigrationClaim(t, client, pvc, "a")
	return client
}
func readyMigrationNode(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
}
func bindMigrationClaim(t *testing.T, c *Client, name, node string) {
	t.Helper()
	ctx := t.Context()
	pvc, err := c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pvc.Spec.VolumeName = "pv-" + name
	pvc.Status.Phase = corev1.ClaimBound
	if _, err = c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Update(ctx, pvc, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: pvc.Spec.VolumeName}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/storage/" + name}}, NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{node}}}}}}}}}
	if _, err = c.Clientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}
func TestMigratedComponentSettingsPreserveClaimAndNode(t *testing.T) {
	for _, component := range []string{InfrastructureLoki, InfrastructureVictoriaMetrics} {
		t.Run(component, func(t *testing.T) {
			c := migrationClient(t, component)
			ctx := t.Context()
			snapshot, err := c.ComponentStorageSnapshot(ctx, component)
			if err != nil {
				t.Fatal(err)
			}
			source, err := c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(ctx, snapshot.PVC, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			target := source.DeepCopy()
			target.Name = "migrated-data"
			target.ResourceVersion = ""
			target.Spec.VolumeName = ""
			target.Status = corev1.PersistentVolumeClaimStatus{}
			if _, err = c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Create(ctx, target, metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
			bindMigrationClaim(t, c, target.Name, "b")
			if err = c.SetComponentStorage(ctx, snapshot, target.Name, "b", 1, false); err != nil {
				t.Fatal(err)
			}
			if component == InfrastructureLoki {
				var status *LoggingStatus
				status, err = c.InstallLoggingContext(ctx, LoggingConfig{NodeName: "b", RetentionDays: 7})
				if err == nil && (status.PVCName != target.Name || status.NodeName != "b" || status.RetentionDays != 7) {
					t.Fatalf("settings reverted storage: %+v", status)
				}
			} else {
				var status *VictoriaMetricsStatus
				status, err = c.InstallVictoriaMetricsContext(ctx, VictoriaMetricsConfig{NodeName: "b", RetentionDays: 7})
				if err == nil && (status.PVCName != target.Name || status.NodeName != "b" || status.RetentionDays != 7) {
					t.Fatalf("settings reverted storage: %+v", status)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			after, err := c.ComponentStorageSnapshot(ctx, component)
			if err != nil || after.PVC != target.Name || after.Node != "b" {
				t.Fatal(after, err)
			}
			if _, err = c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(ctx, snapshot.PVC, metav1.GetOptions{}); err != nil {
				t.Fatal("source claim deleted", err)
			}
			if err = c.SetComponentStorage(ctx, snapshot, snapshot.PVC, "a", 1, true); err != nil {
				t.Fatal(err)
			}
			restored, err := c.ComponentStorageSnapshot(ctx, component)
			if err != nil || restored.PVC != snapshot.PVC || restored.Node != "a" {
				t.Fatal(restored, err)
			}
		})
	}
}
func TestComponentMigrationLockGuardsMutations(t *testing.T) {
	for _, component := range []string{InfrastructureLoki, InfrastructureVictoriaMetrics} {
		t.Run(component, func(t *testing.T) {
			c := migrationClient(t, component)
			ctx := t.Context()
			if err := c.LockComponentMigration(ctx, component, 42); err != nil {
				t.Fatal(err)
			}
			if err := c.LockComponentMigration(ctx, component, 42); err != nil {
				t.Fatal("own lock not idempotent", err)
			}
			if err := c.LockComponentMigration(ctx, component, 43); err == nil {
				t.Fatal("foreign lock acquired")
			}
			if err := c.UnlockComponentMigration(ctx, component, 43); err == nil {
				t.Fatal("foreign lock deleted")
			}
			var err error
			if component == InfrastructureLoki {
				_, err = c.InstallLoggingContext(ctx, LoggingConfig{NodeName: "a", RetentionDays: 7})
				if err == nil {
					t.Fatal("update allowed during migration")
				}
				err = c.UninstallLoggingContext(ctx)
			} else {
				_, err = c.InstallVictoriaMetricsContext(ctx, VictoriaMetricsConfig{NodeName: "a", RetentionDays: 7})
				if err == nil {
					t.Fatal("update allowed during migration")
				}
				err = c.UninstallVictoriaMetricsContext(ctx)
			}
			if err == nil {
				t.Fatal("uninstall allowed during migration")
			}
			if err = c.UnlockComponentMigration(ctx, component, 42); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestSnapshotRejectsExternalOrUnboundClaim(t *testing.T) {
	for _, invalid := range []string{"owner", "pending"} {
		t.Run(invalid, func(t *testing.T) {
			c := migrationClient(t, InfrastructureLoki)
			claim, _ := c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(t.Context(), lokiPVCName, metav1.GetOptions{})
			if invalid == "owner" {
				delete(claim.Labels, InfrastructureLabel)
			} else {
				claim.Status.Phase = corev1.ClaimPending
			}
			c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Update(t.Context(), claim, metav1.UpdateOptions{})
			if _, err := c.ComponentStorageSnapshot(t.Context(), InfrastructureLoki); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}
func TestWaitForStoppedIncludesTerminatingPods(t *testing.T) {
	c := migrationClient(t, InfrastructureLoki)
	snapshot, err := c.ComponentStorageSnapshot(t.Context(), InfrastructureLoki)
	if err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	_, err = c.Clientset.CoreV1().Pods(victoriaMetricsNamespace).Create(t.Context(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "old", Labels: snapshot.StatefulSet.Selector.MatchLabels, DeletionTimestamp: &now}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err = c.WaitComponentStorage(ctx, snapshot, "a", snapshot.PVC, false); err == nil {
		t.Fatal("terminating writer treated as stopped")
	}
}
