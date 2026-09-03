package shared

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
)

var coreDNSForwardPattern = regexp.MustCompile(`(?m)^(\s*forward\s+\.\s+)([^\n{]+)(\{[^\n]*\})?\s*$`)

func ForwardTargets(corefile string) []string {
	matches := coreDNSForwardPattern.FindStringSubmatch(corefile)
	if len(matches) < 3 {
		return []string{}
	}
	return strings.Fields(strings.TrimSpace(matches[2]))
}

func PolicyPayload(policy *model.ClusterDNSPolicy) map[string]any {
	if policy == nil {
		return nil
	}
	var resolvers []string
	_ = json.Unmarshal([]byte(policy.Resolvers), &resolvers)
	return map[string]any{"revision": policy.Revision, "resolvers": resolvers, "created_at": policy.CreatedAt}
}

func CoreDNSPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
