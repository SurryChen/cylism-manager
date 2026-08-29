package registry

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

// ValidateResourceQuantities validates Kubernetes CPU and memory requests and
// limits, including the request <= limit invariant.
func ValidateResourceQuantities(cpuRequest, cpuLimit, memoryRequest, memoryLimit string) error {
	values := []string{cpuRequest, cpuLimit, memoryRequest, memoryLimit}
	for _, value := range values {
		quantity, err := resource.ParseQuantity(value)
		if err != nil || quantity.Sign() <= 0 {
			return fmt.Errorf("资源数量 %q 无效", value)
		}
	}
	cr, _ := resource.ParseQuantity(cpuRequest)
	cl, _ := resource.ParseQuantity(cpuLimit)
	mr, _ := resource.ParseQuantity(memoryRequest)
	ml, _ := resource.ParseQuantity(memoryLimit)
	if cr.Cmp(cl) > 0 || mr.Cmp(ml) > 0 {
		return errors.New("资源 request 不能大于对应 limit")
	}
	return nil
}

// ValidateExistingPVCName validates the reference to an already-created PVC.
// The Registry workflow deliberately does not create or resize PVCs here.
func ValidateExistingPVCName(name string) error {
	if name == "" || len(validation.IsDNS1123Subdomain(name)) > 0 {
		return errors.New("请选择合法的现有 PVC")
	}
	return nil
}

// CertificateDomainsCoverHostname reports whether one certificate SAN covers
// the hostname. Wildcards match exactly one DNS label, as required by TLS.
func CertificateDomainsCoverHostname(domains []string, hostname string) bool {
	for _, domain := range domains {
		if certificateCoversHostname(domain, hostname) {
			return true
		}
	}
	return false
}

func certificateCoversHostname(certificateDomain, hostname string) bool {
	certificateDomain = strings.ToLower(strings.TrimSpace(certificateDomain))
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if certificateDomain == hostname {
		return true
	}
	if !strings.HasPrefix(certificateDomain, "*.") {
		return false
	}
	suffix := strings.TrimPrefix(certificateDomain, "*")
	if !strings.HasSuffix(hostname, suffix) {
		return false
	}
	prefix := strings.TrimSuffix(hostname, suffix)
	return prefix != "" && !strings.Contains(prefix, ".")
}
