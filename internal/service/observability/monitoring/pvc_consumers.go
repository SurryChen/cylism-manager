package monitoring

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// PodReader is the minimal Kubernetes read boundary needed to resolve PVC
// consumers. The monitoring service does not depend on a concrete client.
type PodReader interface {
	ListRuntimePods(context.Context, string, string) ([]corev1.Pod, error)
}

type PVCConsumerReader struct{ Pods PodReader }

func (r PVCConsumerReader) Consumers(ctx context.Context) (map[string][]string, error) {
	if r.Pods == nil {
		return map[string][]string{}, nil
	}
	pods, err := r.Pods.ListRuntimePods(ctx, "", "")
	if err != nil {
		return nil, err
	}
	consumers := make(map[string][]string)
	for _, pod := range pods {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName == "" {
				continue
			}
			key := pod.Namespace + "/" + volume.PersistentVolumeClaim.ClaimName
			consumers[key] = append(consumers[key], pod.Name)
		}
	}
	for key, names := range consumers {
		sort.Strings(names)
		consumers[key] = names
	}
	return consumers, nil
}
