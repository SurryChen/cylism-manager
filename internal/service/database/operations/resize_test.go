package operations

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/database"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResizeDurableTaskPreservesReplicationAndWaitsForNewPod(t *testing.T) {
	s, client := testService(t)
	ctx := context.Background()
	item := model.DatabaseInstance{Name: "replica", Mode: "managed", ClusterID: 1, CPULimit: "1", MemoryLimit: "1Gi", StorageSize: "10Gi"}
	mustCreate(t, s, &item)
	if err := database.Apply(ctx, client, &item, "existing-root", ""); err != nil {
		t.Fatal(err)
	}
	sets := client.Clientset.AppsV1().StatefulSets(database.Namespace)
	set, _ := sets.Get(ctx, database.ResourceName(item.ID), metav1.GetOptions{})
	set.Spec.Template.Spec.Containers[0].Args = append(set.Spec.Template.Spec.Containers[0].Args, "--replicate-do-db=business", "--replicate-wild-do-table=business.%", "--super-read-only=ON", "--max-connections=9000")
	sets.Update(ctx, set, metav1.UpdateOptions{})
	task, err := s.QueueResize(ctx, item.ID, "2", "2Gi")
	if err != nil {
		t.Fatal(err)
	}
	var original model.DatabaseInstance
	s.store.DB().First(&original, item.ID)
	if original.MemoryLimit != "1Gi" {
		t.Fatal("queue changed effective budget before applying")
	}
	if _, err := s.QueueResize(ctx, item.ID, "4", "4Gi"); err == nil {
		t.Fatal("concurrent resize allowed")
	}
	s.Reconcile(ctx)
	set, _ = sets.Get(ctx, database.ResourceName(item.ID), metav1.GetOptions{})
	container := set.Spec.Template.Spec.Containers[0]
	args := strings.Join(container.Args, " ")
	for _, want := range []string{"--replicate-do-db=business", "--replicate-wild-do-table=business.%", "--super-read-only=ON", "--innodb-buffer-pool-size=512M", "--max-connections=50"} {
		if !strings.Contains(args, want) {
			t.Fatalf("lost %s: %s", want, args)
		}
	}
	if strings.Contains(args, "9000") || container.Resources.Limits.Cpu().Cmp(resource.MustParse("2")) != 0 || container.Resources.Limits.Memory().Cmp(resource.MustParse("2Gi")) != 0 {
		t.Fatal("budget or tuning not updated")
	}
	if len(set.Spec.VolumeClaimTemplates) != 1 || container.Env[0].ValueFrom.SecretKeyRef.Name != database.ResourceName(item.ID) {
		t.Fatal("storage or credentials changed")
	}
	s.Reconcile(ctx)
	if got := readTask(t, s, task.ID); got.Status == "succeeded" {
		t.Fatal("finished while old pod still running")
	}
	set.Status.ObservedGeneration = set.Generation
	set.Status.ReadyReplicas = 1
	set.Status.UpdatedReplicas = 1
	set.Status.CurrentRevision = "new"
	set.Status.UpdateRevision = "new"
	sets.UpdateStatus(ctx, set, metav1.UpdateOptions{})
	client.Clientset.CoreV1().Pods(database.Namespace).Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: database.ResourceName(item.ID) + "-0", Namespace: database.Namespace, Annotations: map[string]string{database.ResizeAnnotation: "old"}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "mysql", Ready: true}}}}, metav1.CreateOptions{})
	// Controller ready counts alone must not accept an old Pod.
	ready, err := database.InstanceResizeReady(ctx, client, item.ID, fmt.Sprint(task.ID))
	if err != nil || ready {
		t.Fatalf("old pod accepted: %v %v", ready, err)
	}
	pod, _ := client.Clientset.CoreV1().Pods(database.Namespace).Get(ctx, database.ResourceName(item.ID)+"-0", metav1.GetOptions{})
	pod.Annotations[database.ResizeAnnotation] = fmt.Sprint(task.ID)
	client.Clientset.CoreV1().Pods(database.Namespace).Update(ctx, pod, metav1.UpdateOptions{})
	restarted := New(s.store, s.key, s.client)
	restarted.Reconcile(ctx)
	if got := readTask(t, s, task.ID); got.Status != "succeeded" {
		t.Fatalf("did not resume: %+v", got)
	}
	if err := AssertUnlocked(s.store.DB(), Reference(item.ID)); err != nil {
		t.Fatal(err)
	}
	var after model.DatabaseInstance
	s.store.DB().First(&after, item.ID)
	if after.MemoryLimit != "2Gi" || after.Status != "ready" {
		t.Fatalf("bad saved budget: %+v", after)
	}
	jobs, _ := client.Clientset.BatchV1().Jobs(database.Namespace).List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Fatal("resize created a tooling Job")
	}
}

func TestResizeRejectsInvalidAndExternalInstances(t *testing.T) {
	s, _ := testService(t)
	item := model.DatabaseInstance{Name: "rds", Mode: "external"}
	mustCreate(t, s, &item)
	for _, values := range [][2]string{{"", ""}, {"2", "256Mi"}, {"0", "2Gi"}, {"2", "2Gi"}} {
		if _, err := s.QueueResize(context.Background(), item.ID, values[0], values[1]); err == nil {
			t.Fatal("invalid resize allowed")
		}
	}
	var count int64
	s.store.DB().Model(&model.DatabaseOperation{}).Count(&count)
	if count != 0 {
		t.Fatal("rejected resize queued task")
	}
}

func TestCancelResizeKeepsAppliedBudgetAndNeverDeletesDatabase(t *testing.T) {
	s, client := testService(t)
	ctx := context.Background()
	item := model.DatabaseInstance{Name: "replica", Mode: "managed", ClusterID: 1, CPULimit: "1", MemoryLimit: "1Gi", StorageSize: "10Gi"}
	mustCreate(t, s, &item)
	if err := database.Apply(ctx, client, &item, "existing-root", ""); err != nil {
		t.Fatal(err)
	}
	task, err := s.QueueResize(ctx, item.ID, "2", "2Gi")
	if err != nil {
		t.Fatal(err)
	}
	s.Reconcile(ctx)
	if err := s.Control(task.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	s.Reconcile(ctx)
	if got := readTask(t, s, task.ID); got.Status != "cancelled" || !strings.Contains(got.Progress, "不会回滚") {
		t.Fatalf("bad cancellation: %+v", got)
	}
	set, err := client.Clientset.AppsV1().StatefulSets(database.Namespace).Get(ctx, database.ResourceName(item.ID), metav1.GetOptions{})
	if err != nil {
		t.Fatal("cancellation deleted database")
	}
	if set.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().Cmp(resource.MustParse("2Gi")) != 0 {
		t.Fatal("cancellation rolled back applied budget")
	}
	if err := AssertUnlocked(s.store.DB(), Reference(item.ID)); err != nil {
		t.Fatal(err)
	}
}
