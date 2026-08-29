package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const RegistryProxyNamespace = "kube-system"

// RegistryProxyReconciler owns Kubernetes resource lifecycle operations for a
// non-persistent Registry pull-through proxy. It receives decrypted runtime
// environment only from the service boundary and never persists credentials.
type RegistryProxyReconciler struct{ client *Client }

// RegistryProxyDiagnostic is the bounded result of checking the configured
// upstream from a ready Registry proxy Pod.
type RegistryProxyDiagnostic struct {
	Status      string   `json:"status"`
	ResolvedIPs []string `json:"resolved_ips"`
	HTTPStatus  string   `json:"http_status,omitempty"`
	ElapsedMS   int64    `json:"elapsed_ms"`
	Summary     string   `json:"summary"`
}

func NewRegistryProxyReconciler(client *Client) *RegistryProxyReconciler {
	return &RegistryProxyReconciler{client: client}
}

func (r *RegistryProxyReconciler) Available() bool {
	return r != nil && r.client != nil && r.client.Clientset != nil
}

func (r *RegistryProxyReconciler) EnsureNode(ctx context.Context, nodeName string) error {
	if !r.Available() {
		return errors.New("Kubernetes 集群未连接")
	}
	if _, err := r.client.Clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{}); err != nil {
		return errors.New("部署节点不存在或未加入集群")
	}
	return nil
}

func (r *RegistryProxyReconciler) Apply(ctx context.Context, proxy *model.RegistryProxy, environment map[string]string) error {
	if !r.Available() {
		return errors.New("Kubernetes 集群未连接")
	}
	resourceName := registryProxyResourceName(proxy)
	labels := map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "app.kubernetes.io/name": resourceName, "cylism.io/registry": proxy.Registry}
	cacheLimit := resource.MustParse(strconv.Itoa(int(proxy.CacheLimitGi)) + "Gi")
	replicas := int32(1)
	env := []corev1.EnvVar{{Name: "REGISTRY_PROXY_REMOTEURL", Value: environment["REGISTRY_PROXY_REMOTEURL"]}, {Name: "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", Value: environment["REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY"]}}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		if value := environment[name]; value != "" {
			env = append(env, corev1.EnvVar{Name: name, Value: value})
		}
	}
	podSpec := corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: proxy.NodeName}, Volumes: []corev1.Volume{{Name: "cache", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &cacheLimit}}}}, Containers: []corev1.Container{{Name: "registry", Image: "registry:2.8", Ports: []corev1.ContainerPort{{ContainerPort: 5000}}, Env: env, VolumeMounts: []corev1.VolumeMount{{Name: "cache", MountPath: "/var/lib/registry"}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi")}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/v2/", Port: intstr.FromInt(5000)}}, InitialDelaySeconds: 3, PeriodSeconds: 5}}}}
	if dnsServers := registryProxyDNSServers(proxy); len(dnsServers) > 0 {
		ndots := "1"
		podSpec.DNSPolicy = corev1.DNSNone
		podSpec.DNSConfig = &corev1.PodDNSConfig{Nameservers: dnsServers, Options: []corev1.PodDNSConfigOption{{Name: "ndots", Value: &ndots}}}
	}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: RegistryProxyNamespace, Labels: labels}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: podSpec}}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: RegistryProxyNamespace, Labels: labels}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: labels, Ports: []corev1.ServicePort{{Name: "registry", Port: 5000, TargetPort: intstr.FromInt(5000), NodePort: proxy.NodePort}}}}
	if current, err := r.client.Clientset.AppsV1().Deployments(RegistryProxyNamespace).Get(ctx, resourceName, metav1.GetOptions{}); err == nil {
		deployment.ResourceVersion = current.ResourceVersion
		if _, err := r.client.Clientset.AppsV1().Deployments(RegistryProxyNamespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	} else if _, err := r.client.Clientset.AppsV1().Deployments(RegistryProxyNamespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		return err
	}
	if current, err := r.client.Clientset.CoreV1().Services(RegistryProxyNamespace).Get(ctx, resourceName, metav1.GetOptions{}); err == nil {
		service.ResourceVersion = current.ResourceVersion
		_, err = r.client.Clientset.CoreV1().Services(RegistryProxyNamespace).Update(ctx, service, metav1.UpdateOptions{})
		return err
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	_, err := r.client.Clientset.CoreV1().Services(RegistryProxyNamespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

func (r *RegistryProxyReconciler) ClearCache(ctx context.Context, proxy *model.RegistryProxy) error {
	if !r.Available() {
		return errors.New("Kubernetes 集群未连接")
	}
	return r.client.Clientset.CoreV1().Pods(RegistryProxyNamespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + registryProxyResourceName(proxy)})
}

func (r *RegistryProxyReconciler) DeploymentAvailable(ctx context.Context, proxy *model.RegistryProxy) (bool, bool, error) {
	if !r.Available() {
		return false, false, errors.New("Kubernetes 集群未连接")
	}
	deployment, err := r.client.Clientset.AppsV1().Deployments(RegistryProxyNamespace).Get(ctx, registryProxyResourceName(proxy), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	return deployment.Status.AvailableReplicas > 0, false, nil
}

func (r *RegistryProxyReconciler) DeleteLegacyResources(ctx context.Context, resourceName string) error {
	if !r.Available() {
		return errors.New("Kubernetes 集群未连接")
	}
	if err := r.client.Clientset.CoreV1().Services(RegistryProxyNamespace).Delete(ctx, resourceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("删除旧代理 Service 失败: %w", err)
	}
	if err := r.client.Clientset.AppsV1().Deployments(RegistryProxyNamespace).Delete(ctx, resourceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("删除旧代理 Deployment 失败: %w", err)
	}
	return nil
}

// DiagnoseUpstream probes only the upstream configured on the managed proxy.
// The Pod command is fixed; callers cannot provide an arbitrary network target
// or shell arguments.
func (r *RegistryProxyReconciler) DiagnoseUpstream(ctx context.Context, proxy *model.RegistryProxy) (RegistryProxyDiagnostic, error) {
	if !r.Available() || r.client.Config == nil {
		return RegistryProxyDiagnostic{}, errors.New("Kubernetes exec unavailable")
	}
	pods, err := r.client.Clientset.CoreV1().Pods(RegistryProxyNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + registryProxyResourceName(proxy)})
	if err != nil {
		return RegistryProxyDiagnostic{}, err
	}
	for index := range pods.Items {
		pod := &pods.Items[index]
		if !registryProxyPodReady(pod) {
			continue
		}
		return r.execDiagnostic(ctx, pod.Name, proxy.UpstreamURL)
	}
	return RegistryProxyDiagnostic{Status: "proxy_not_ready", Summary: "镜像代理没有就绪的 Pod"}, nil
}

func (r *RegistryProxyReconciler) execDiagnostic(ctx context.Context, podName, upstream string) (RegistryProxyDiagnostic, error) {
	parsed, err := url.Parse(upstream)
	if err != nil || parsed.Hostname() == "" {
		return RegistryProxyDiagnostic{}, fmt.Errorf("invalid upstream")
	}
	request := r.client.Clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace(RegistryProxyNamespace).Name(podName).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: "registry", Command: registryProxyDiagnosticCommand(parsed.Hostname()), Stdout: true, Stderr: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(r.client.Config, http.MethodPost, request.URL())
	if err != nil {
		return RegistryProxyDiagnostic{}, err
	}
	var stdout, stderr bytes.Buffer
	started := time.Now()
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr}); err != nil {
		return RegistryProxyDiagnostic{Status: "diagnostic_failed", ElapsedMS: time.Since(started).Milliseconds(), Summary: "代理 Pod 内探测命令失败"}, nil
	}
	return parseRegistryProxyDiagnostic(stdout.String(), time.Since(started).Milliseconds()), nil
}

func registryProxyPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func registryProxyDiagnosticCommand(host string) []string {
	return []string{"sh", "-c", `host=$1; started=$(date +%s%3N); ips=$(getent ahostsv4 "$host" 2>/dev/null | awk '{print $1}' | sort -u | head -8 | tr '\n' ','); tool=missing; output=; http=; if command -v curl >/dev/null 2>&1; then tool=curl; output=$(curl -ksS -o /dev/null -w '%{http_code}' --connect-timeout 5 --max-time 10 "https://$host/v2/" 2>&1); http=$output; elif command -v wget >/dev/null 2>&1; then tool=wget; output=$(wget -S --no-check-certificate -T 10 -t 1 -O /dev/null "https://$host/v2/" 2>&1); http=$(printf '%s\n' "$output" | awk '/^  HTTP\// {code=$2} /^HTTP\// {code=$2} END {print code}'); fi; status=upstream_http_error; if [ "$tool" = missing ]; then status=command_missing; elif [ -z "$ips" ]; then status=dns_resolution_failed; elif [ "$http" = 200 ] || [ "$http" = 401 ]; then status=healthy; elif printf '%s' "$output" | grep -qiE 'timed out|connection timed out'; then status=upstream_connect_timeout; elif printf '%s' "$output" | grep -qiE 'certificate|tls|ssl'; then status=upstream_tls_failed; fi; elapsed=$(( $(date +%s%3N) - started )); printf 'status=%s;tool=%s;ips=%s;http=%s;elapsed=%s\n' "$status" "$tool" "$ips" "$http" "$elapsed"`, "diagnose", host}
}

func parseRegistryProxyDiagnostic(output string, fallbackElapsed int64) RegistryProxyDiagnostic {
	result := RegistryProxyDiagnostic{Status: "diagnostic_failed", ElapsedMS: fallbackElapsed, Summary: "代理 Pod 未返回有效诊断结果"}
	for _, field := range strings.Split(strings.TrimSpace(output), ";") {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "status":
			result.Status = strings.TrimSpace(parts[1])
		case "ips":
			for _, ip := range strings.Split(strings.TrimSuffix(parts[1], ","), ",") {
				if net.ParseIP(ip) != nil {
					result.ResolvedIPs = append(result.ResolvedIPs, ip)
				}
			}
		case "http":
			result.HTTPStatus = strings.TrimSpace(parts[1])
		case "elapsed":
			if value, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil && value >= 0 {
				result.ElapsedMS = value
			}
		}
	}
	switch result.Status {
	case "healthy":
		result.Summary = "代理 Pod 可访问上游 Registry"
	case "dns_resolution_failed":
		result.Summary = "代理 Pod 未能解析上游 Registry"
	case "upstream_connect_timeout":
		result.Summary = "上游 Registry 连接超时或不可达"
	case "upstream_tls_failed":
		result.Summary = "上游 Registry TLS 握手或证书校验失败"
	case "command_missing":
		result.Summary = "代理镜像缺少可用的 HTTP 诊断命令"
	case "upstream_http_error":
		result.Summary = "上游 Registry 返回异常 HTTP 状态"
	default:
		result.Status, result.Summary = "diagnostic_failed", "代理 Pod 未返回有效诊断结果"
	}
	return result
}

func registryProxyResourceName(proxy *model.RegistryProxy) string {
	if name := strings.TrimSpace(proxy.ResourceName); name != "" {
		return name
	}
	return "cylism-registry-proxy-" + strconv.Itoa(int(proxy.ID))
}

func registryProxyDNSServers(proxy *model.RegistryProxy) []string {
	var servers []string
	_ = json.Unmarshal([]byte(proxy.DNSResolvers), &servers)
	return servers
}
