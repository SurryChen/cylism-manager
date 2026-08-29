// Package registry contains business rules shared by Registry-facing API and
// automation entry points. It deliberately has no HTTP or Kubernetes dependency.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/google/go-containerregistry/pkg/name"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	ManagedRegistryNamespace    = "cylism-system"
	ManagedRegistryResourceName = "cylism-oci-registry"
)

// ManagedRegistryInput is the transport-neutral configuration required to
// create or update the platform-managed Registry.
type ManagedRegistryInput struct {
	Name                string
	Namespace           string
	Endpoint            string
	VerificationImage   string
	RegistryImage       string
	DataNode            string
	PVCName             string
	CPURequest          string
	CPULimit            string
	MemoryRequest       string
	MemoryLimit         string
	InsecureHTTP        bool
	ConfirmInsecureHTTP bool
	CertificateName     string
	PullUsername        string
	PullPassword        string
}

// NormalizeEndpoint validates and canonicalizes a Registry endpoint. Registry
// endpoints are hostnames with an optional port; schemes, paths and IP
// literals are intentionally rejected because they cannot be used consistently
// by the platform's Ingress and node mirror configuration.
func NormalizeEndpoint(value string) (endpoint, host string, err error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "://") || strings.ContainsAny(value, "/?#@") {
		return "", "", errors.New("制品库地址只支持主机名和可选端口")
	}
	parsed, parseErr := url.Parse("https://" + value)
	if parseErr != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("制品库地址无效")
	}
	host = parsed.Hostname()
	if host == "" || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return "", "", errors.New("制品库地址必须使用可解析的内网域名")
	}
	if port := parsed.Port(); port != "" {
		number, portErr := strconv.Atoi(port)
		if portErr != nil || number < 1 || number > 65535 {
			return "", "", errors.New("制品库端口无效")
		}
	}
	return strings.ToLower(value), strings.ToLower(host), nil
}

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

// BuildManagedRegistry validates input and returns a normalized registry and
// its ingress hostname. Current is supplied for update operations.
func BuildManagedRegistry(input ManagedRegistryInput, current *model.ManagedOCIRegistry) (*model.ManagedOCIRegistry, string, error) {
	name := strings.TrimSpace(input.Name)
	if namespace := strings.TrimSpace(input.Namespace); namespace != "" && namespace != ManagedRegistryNamespace {
		return nil, "", fmt.Errorf("制品库命名空间固定为 %s", ManagedRegistryNamespace)
	}
	endpoint, host, err := NormalizeEndpoint(input.Endpoint)
	if err != nil {
		return nil, "", err
	}
	image, node, pvcName := strings.TrimSpace(input.RegistryImage), strings.TrimSpace(input.DataNode), strings.TrimSpace(input.PVCName)
	username, password := strings.TrimSpace(input.PullUsername), strings.TrimSpace(input.PullPassword)
	if name == "" || len(name) > 128 {
		return nil, "", errors.New("制品库名称必填，且长度不能超过限制")
	}
	if image == "" || strings.ContainsAny(image, " \t\r\n") {
		return nil, "", errors.New("Registry 镜像地址无效")
	}
	if pvcName == "" || len(validation.IsDNS1123Subdomain(pvcName)) > 0 {
		return nil, "", errors.New("请选择合法的现有 PVC")
	}
	if username == "" || strings.ContainsAny(username, ":\r\n") {
		return nil, "", errors.New("拉取账号必填，且不能包含冒号或换行")
	}
	if (current == nil && len(password) < 12) || (current != nil && password != "" && len(password) < 12) {
		return nil, "", errors.New("拉取密码至少需要 12 个字符")
	}
	if input.InsecureHTTP && !input.ConfirmInsecureHTTP {
		return nil, "", errors.New("使用 HTTP 制品库必须明确确认明文镜像和凭据传输风险")
	}
	certificateName := strings.TrimSpace(input.CertificateName)
	if !input.InsecureHTTP && certificateName == "" {
		return nil, "", errors.New("HTTPS 制品库必须选择已就绪的匹配证书")
	}
	if input.InsecureHTTP {
		certificateName = ""
	}
	cpuRequest, cpuLimit := strings.TrimSpace(input.CPURequest), strings.TrimSpace(input.CPULimit)
	memoryRequest, memoryLimit := strings.TrimSpace(input.MemoryRequest), strings.TrimSpace(input.MemoryLimit)
	if cpuRequest == "" {
		cpuRequest = "100m"
	}
	if cpuLimit == "" {
		cpuLimit = "500m"
	}
	if memoryRequest == "" {
		memoryRequest = "256Mi"
	}
	if memoryLimit == "" {
		memoryLimit = "1Gi"
	}
	if err := ValidateResourceQuantities(cpuRequest, cpuLimit, memoryRequest, memoryLimit); err != nil {
		return nil, "", err
	}
	registry := &model.ManagedOCIRegistry{Name: name, Namespace: ManagedRegistryNamespace, ResourceName: ManagedRegistryResourceName, Endpoint: endpoint, RegistryImage: image, DataNode: node, PVCName: pvcName, CPURequest: cpuRequest, CPULimit: cpuLimit, MemoryRequest: memoryRequest, MemoryLimit: memoryLimit, InsecureHTTP: input.InsecureHTTP, CertificateName: certificateName, PullUsername: username, Status: "pending", VerificationImage: strings.TrimSpace(input.VerificationImage)}
	if registry.VerificationImage == "" {
		return nil, "", errors.New("验证镜像必填")
	}
	if err := ValidateVerificationImage(endpoint, registry.VerificationImage); err != nil {
		return nil, "", errors.New(strings.Replace(err.Error(), "当前 Registry", "制品库地址", 1))
	}
	if current != nil {
		registry.ID, registry.CreatedAt, registry.CreatedBy, registry.EncryptedCredential = current.ID, current.CreatedAt, current.CreatedBy, current.EncryptedCredential
		registry.ImageRegistryID, registry.NodeRegistryMirrorID = current.ImageRegistryID, current.NodeRegistryMirrorID
		if registry.PVCName != current.PVCName || (node != "" && node != current.DataNode) {
			return nil, "", errors.New("数据节点和 PVC 创建后不可修改")
		}
	}
	return registry, host, nil
}

// ManagedRegistryAssociations builds the image Registry and node-mirror
// records owned by a managed Registry. They share one lifecycle and therefore
// must be created and updated together by the persistence adapter.
func ManagedRegistryAssociations(registry *model.ManagedOCIRegistry) (*model.ImageRegistry, *model.NodeRegistryMirror) {
	scheme := "https"
	if registry.InsecureHTTP {
		scheme = "http"
	}
	endpoints, _ := json.Marshal([]string{scheme + "://" + registry.Endpoint})
	name := "受管制品库 · " + registry.Name
	return &model.ImageRegistry{Name: name, Endpoint: registry.Endpoint, VerificationImage: registry.VerificationImage, AuthType: "basic", Username: registry.PullUsername, Credential: registry.EncryptedCredential, Enabled: true, CreatedBy: registry.CreatedBy, ManagedRegistryID: &registry.ID}, &model.NodeRegistryMirror{Name: name, Registry: registry.Endpoint, Endpoints: string(endpoints), VerificationImage: registry.VerificationImage, Username: registry.PullUsername, Credential: registry.EncryptedCredential, Enabled: true, CreatedBy: registry.CreatedBy, ManagedRegistryID: &registry.ID}
}

// ValidateVerificationImage ensures the image has an explicit reference and
// belongs to the configured Registry.
func ValidateVerificationImage(registry, image string) error {
	if strings.TrimSpace(image) == "" {
		return errors.New("验证镜像必填")
	}
	ref, err := name.ParseReference(strings.TrimSpace(image))
	if err != nil {
		return errors.New("验证镜像格式无效，请填写完整镜像地址和标签")
	}
	configuredRegistry, err := name.NewRegistry(strings.TrimSpace(registry))
	if err != nil {
		return errors.New("Registry 地址无效")
	}
	if !strings.EqualFold(ref.Context().RegistryStr(), configuredRegistry.RegistryStr()) {
		return errors.New("验证镜像必须属于当前 Registry")
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
