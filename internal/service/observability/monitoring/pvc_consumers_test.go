package monitoring

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakePodReader struct{ pods []corev1.Pod }

func (f fakePodReader) ListRuntimePods(context.Context, string, string) ([]corev1.Pod, error) {
	return f.pods, nil
}

func TestPVCConsumerReaderGroupsAndSortsPods(t *testing.T) {
	r := PVCConsumerReader{Pods: fakePodReader{pods: []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "z", Namespace: "ns"}, Spec: corev1.PodSpec{Volumes: []corev1.Volume{{VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}, Spec: corev1.PodSpec{Volumes: []corev1.Volume{{VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}}}}},
	}}}
	got, err := r.Consumers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got["ns/data"]) != 2 || got["ns/data"][0] != "a" {
		t.Fatalf("unexpected consumers: %#v", got)
	}
}
