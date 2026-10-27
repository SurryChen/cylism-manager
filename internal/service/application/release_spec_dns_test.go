package application

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestReleaseDNSDefaultsAndSnapshotRendering(t *testing.T) {
	for _, kind := range []string{WorkloadKindDeployment, WorkloadKindStatefulSet} {
		for _, hostNetwork := range []bool{false, true} {
			for _, value := range []int{-1, 0, 1, 15} {
				t.Run(kind+"/host="+strconv.FormatBool(hostNetwork)+"/ndots="+strconv.Itoa(value), func(t *testing.T) {
					spec := validTestReleaseSpec()
					spec.HostNetwork = hostNetwork
					if value >= 0 {
						spec.DNSNDots = &value
					}
					// Exercise the same sanitized JSON format used by release history,
					// retries and rollbacks, including an explicit zero override.
					data, err := json.Marshal(SanitizeReleaseSpec(spec))
					if err != nil {
						t.Fatal(err)
					}
					if value < 0 && strings.Contains(string(data), "dns_ndots") {
						t.Fatalf("default DNS must be omitted: %s", data)
					}
					var snapshot ReleaseSpec
					if err := json.Unmarshal(data, &snapshot); err != nil {
						t.Fatal(err)
					}
					resources, err := RenderResources(ApplicationContext{ProjectName: "p", EnvironmentName: "e", ApplicationName: "app", Namespace: "ns", WorkloadKind: kind}, snapshot)
					if err != nil {
						t.Fatal(err)
					}
					var pod corev1.PodSpec
					if kind == WorkloadKindDeployment {
						pod = resources.Deployment.Spec.Template.Spec
					} else {
						pod = resources.StatefulSet.Spec.Template.Spec
					}
					if value < 0 {
						if pod.DNSConfig != nil {
							t.Fatalf("default must inherit cluster DNS: %+v", pod.DNSConfig)
						}
					} else {
						if pod.DNSConfig == nil || len(pod.DNSConfig.Options) != 1 {
							t.Fatalf("missing DNS override: %+v", pod.DNSConfig)
						}
						option := pod.DNSConfig.Options[0]
						if option.Name != "ndots" || option.Value == nil || *option.Value != strconv.Itoa(value) {
							t.Fatalf("wrong DNS override: %+v", option)
						}
					}
					if hostNetwork && pod.DNSPolicy != corev1.DNSClusterFirstWithHostNet {
						t.Fatalf("host network lost cluster DNS: %s", pod.DNSPolicy)
					}
				})
			}
		}
	}
}

func TestValidateReleaseDNSNDots(t *testing.T) {
	for _, value := range []int{-1, 0, 1, 15, 16} {
		spec := validTestReleaseSpec()
		spec.DNSNDots = &value
		issues := ValidateReleaseSpec(spec)
		invalid := value < 0 || value > 15
		if !invalid && len(issues) != 0 {
			t.Fatalf("ndots=%d: unexpected issues: %+v", value, issues)
		}
		if invalid && (len(issues) != 1 || issues[0].Field != "dns_ndots") {
			t.Fatalf("ndots=%d: expected DNS validation error: %+v", value, issues)
		}
	}
}
