package api

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
)

// These pure CoreDNS response helpers are also used by the Runtime Agent API.
const (
	coreDNSNamespace = "kube-system"
	coreDNSConfigMap = "coredns"
)

var coreDNSForwardPattern = regexp.MustCompile(`(?m)^(\s*forward\s+\.\s+)([^\n{]+)(\{[^\n]*\})?\s*$`)

func forwardTargets(corefile string) []string {
	matches := coreDNSForwardPattern.FindStringSubmatch(corefile)
	if len(matches) < 3 {
		return []string{}
	}
	return strings.Fields(strings.TrimSpace(matches[2]))
}

func policyPayload(policy *model.ClusterDNSPolicy) any {
	if policy == nil {
		return nil
	}
	var resolvers []string
	_ = json.Unmarshal([]byte(policy.Resolvers), &resolvers)
	return gin.H{"revision": policy.Revision, "resolvers": resolvers, "created_at": policy.CreatedAt}
}

func coreDNSPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
