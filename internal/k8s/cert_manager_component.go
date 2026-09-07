package k8s

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	CertManagerStateReady        = "ready"
	CertManagerStateNotInstalled = "not_installed"
	CertManagerStateInstalling   = "installing"
	CertManagerStateDegraded     = "degraded"
	CertManagerStateUnauthorized = "unauthorized"
	CertManagerStateUnavailable  = "unavailable"

	certManagerNamespace = "cert-manager"
	certManagerHelmName  = "cylism-cert-manager"
)

var helmChartGVR = schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}

// CertManagerStatus describes the prerequisite state required by certificate management.
type CertManagerStatus struct {
	State              string `json:"state"`
	Message            string `json:"message"`
	CertificateCRD     bool   `json:"certificate_crd"`
	IssuerCRD          bool   `json:"issuer_crd"`
	ClusterIssuerCRD   bool   `json:"cluster_issuer_crd"`
	ControllerReady    bool   `json:"controller_ready"`
	WebhookReady       bool   `json:"webhook_ready"`
	CAInjectorReady    bool   `json:"ca_injector_ready"`
	InstallerAvailable bool   `json:"installer_available"`
	InstallationName   string `json:"installation_name,omitempty"`
}

// DNSProviderStatus describes the installation prerequisite for a DNS-01 provider.
type DNSProviderStatus struct {
	Provider         string `json:"provider"`
	State            string `json:"state"`
	Message          string `json:"message"`
	Ready            bool   `json:"ready"`
	InstallationName string `json:"installation_name,omitempty"`
}

func (c *Client) DNSProviderStatusContext(ctx context.Context, providerID string) *DNSProviderStatus {
	status := &DNSProviderStatus{Provider: providerID, State: CertManagerStateUnavailable, Message: "Kubernetes 客户端未初始化"}
	if err := ctx.Err(); err != nil {
		status.Message = err.Error()
		return status
	}
	provider, ok := GetDNSProvider(providerID)
	if !ok {
		status.Message = "不支持的 DNS Provider: " + providerID
		return status
	}
	chart := provider.WebhookChart()
	if chart == nil {
		status.State, status.Message, status.Ready = CertManagerStateReady, "Provider 使用 cert-manager 内置 DNS-01 solver", true
		return status
	}
	status.InstallationName = chart.ReleaseName
	if c == nil || c.Clientset == nil {
		return status
	}
	base := c.CertManagerStatusContext(ctx)
	if base.State != CertManagerStateReady {
		status.State, status.Message = base.State, "cert-manager 未就绪: "+base.Message
		return status
	}
	if !c.helmChartInstallerAvailableContext(ctx) {
		status.Message = "当前集群未提供 K3s HelmChart 安装器"
		return status
	}
	exists, failed, detail := c.helmChartStateContext(ctx, chart.ReleaseName)
	if !exists {
		status.State, status.Message = CertManagerStateNotInstalled, provider.Info().Name+" Webhook 尚未安装"
		return status
	}
	if failed {
		status.State, status.Message = CertManagerStateDegraded, provider.Info().Name+" Webhook 安装失败: "+detail
		return status
	}
	if deploymentReady(ctx, c, chart.ReleaseName) {
		status.State, status.Message, status.Ready = CertManagerStateReady, provider.Info().Name+" Webhook 已就绪", true
		return status
	}
	status.State, status.Message = CertManagerStateInstalling, provider.Info().Name+" Webhook 正在安装，等待控制组件就绪"
	return status
}

func (c *Client) InstallDNSProviderContext(ctx context.Context, providerID string) (*DNSProviderStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	status := c.DNSProviderStatusContext(ctx, providerID)
	provider, ok := GetDNSProvider(providerID)
	if !ok {
		return status, fmt.Errorf("%s", status.Message)
	}
	chart := provider.WebhookChart()
	if chart == nil || status.Ready || status.State == CertManagerStateInstalling {
		return status, nil
	}
	if status.State != CertManagerStateNotInstalled {
		return status, fmt.Errorf("%s", status.Message)
	}
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return status, err
	}
	object := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "helm.cattle.io/v1", "kind": "HelmChart", "metadata": map[string]interface{}{"name": chart.ReleaseName, "namespace": "kube-system", "labels": map[string]interface{}{"app.kubernetes.io/managed-by": "cylism-manager", "cylism.io/dns-provider": providerID}}, "spec": map[string]interface{}{"chart": chart.Chart, "repo": chart.Repository, "version": chart.Version, "targetNamespace": certManagerNamespace, "createNamespace": true, "valuesContent": chart.Values}}}
	_, err = dynamicClient.Resource(helmChartGVR).Namespace("kube-system").Create(ctx, object, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return status, fmt.Errorf("创建 %s Webhook HelmChart: %w", provider.Info().Name, err)
	}
	status.State, status.Message = CertManagerStateInstalling, provider.Info().Name+" Webhook 安装任务已创建，等待 Helm 控制器完成安装"
	return status, nil
}

func (c *Client) CertManagerStatusContext(ctx context.Context) *CertManagerStatus {
	status := &CertManagerStatus{State: CertManagerStateUnavailable, Message: "Kubernetes 客户端未初始化"}
	if err := ctx.Err(); err != nil {
		status.Message = err.Error()
		return status
	}
	if c == nil || c.Clientset == nil {
		return status
	}

	var err error
	status.CertificateCRD, err = c.CheckCRDContext(ctx, "certificates.cert-manager.io")
	if err != nil {
		return certManagerStatusForError(status, "检查 Certificate CRD", err)
	}
	status.IssuerCRD, err = c.CheckCRDContext(ctx, "issuers.cert-manager.io")
	if err != nil {
		return certManagerStatusForError(status, "检查 Issuer CRD", err)
	}
	status.ClusterIssuerCRD, err = c.CheckCRDContext(ctx, "clusterissuers.cert-manager.io")
	if err != nil {
		return certManagerStatusForError(status, "检查 ClusterIssuer CRD", err)
	}
	if !status.CertificateCRD || !status.IssuerCRD || !status.ClusterIssuerCRD {
		status.InstallerAvailable = c.helmChartInstallerAvailableContext(ctx)
		if exists, failed, detail := c.certManagerHelmChartStateContext(ctx); exists {
			status.InstallationName = certManagerHelmName
			if failed {
				status.State = CertManagerStateDegraded
				status.Message = "cert-manager 安装失败: " + detail
				return status
			}
			status.State = CertManagerStateInstalling
			status.Message = "cert-manager 正在安装，等待 CRD 和控制组件就绪"
			return status
		}
		status.State = CertManagerStateNotInstalled
		status.Message = "未检测到完整的 cert-manager CRD"
		return status
	}

	if err := c.checkCertManagerAccessContext(ctx); err != nil {
		return certManagerStatusForError(status, "验证 cert-manager 访问权限", err)
	}
	status.ControllerReady = deploymentReady(ctx, c, certManagerHelmName)
	status.WebhookReady = deploymentReady(ctx, c, certManagerHelmName+"-webhook")
	status.CAInjectorReady = deploymentReady(ctx, c, certManagerHelmName+"-cainjector")
	if !status.ControllerReady || !status.WebhookReady || !status.CAInjectorReady {
		if exists, failed, detail := c.certManagerHelmChartStateContext(ctx); exists && !failed {
			status.State = CertManagerStateInstalling
			status.InstallationName = certManagerHelmName
			status.Message = "cert-manager 正在安装，等待控制组件就绪"
			return status
		} else if exists {
			status.Message = "cert-manager 安装失败: " + detail
			status.State = CertManagerStateDegraded
			return status
		}
		status.State = CertManagerStateDegraded
		status.Message = "cert-manager CRD 已安装，但控制组件尚未全部就绪"
		return status
	}
	status.State = CertManagerStateReady
	status.Message = "cert-manager 已就绪"
	return status
}

// InstallCertManagerContext creates a fixed K3s HelmChart and returns immediately while the Helm controller installs it.
func (c *Client) InstallCertManagerContext(ctx context.Context, repository, chartName, chartVersion string) (*CertManagerStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	status := c.CertManagerStatusContext(ctx)
	if status.State == CertManagerStateReady || status.State == CertManagerStateInstalling {
		return status, nil
	}
	if status.State == CertManagerStateUnauthorized || status.State == CertManagerStateUnavailable {
		return status, fmt.Errorf("%s", status.Message)
	}
	if status.CertificateCRD || status.IssuerCRD || status.ClusterIssuerCRD {
		return status, fmt.Errorf("检测到不完整的 cert-manager 安装，请先修复现有组件")
	}
	if !status.InstallerAvailable {
		return status, fmt.Errorf("当前集群未提供 K3s HelmChart 安装器")
	}

	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return status, err
	}
	chart := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "helm.cattle.io/v1",
		"kind":       "HelmChart",
		"metadata": map[string]interface{}{
			"name":      certManagerHelmName,
			"namespace": "kube-system",
			"labels": map[string]interface{}{
				"app.kubernetes.io/managed-by": "cylism-manager",
			},
		},
		"spec": map[string]interface{}{
			"chart":           chartName,
			"repo":            repository,
			"version":         chartVersion,
			"targetNamespace": certManagerNamespace,
			"createNamespace": true,
			"valuesContent":   "crds:\n  enabled: true\nprometheus:\n  enabled: false\n",
		},
	}}
	_, err = dynamicClient.Resource(helmChartGVR).Namespace("kube-system").Create(ctx, chart, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return status, fmt.Errorf("创建 cert-manager HelmChart: %w", err)
	}
	status.State = CertManagerStateInstalling
	status.InstallationName = certManagerHelmName
	status.Message = "cert-manager 安装任务已创建，等待 Helm 控制器完成安装"
	return status, nil
}

func certManagerStatusForError(status *CertManagerStatus, action string, err error) *CertManagerStatus {
	status.Message = fmt.Sprintf("%s: %v", action, err)
	if apierrors.IsForbidden(err) {
		status.State = CertManagerStateUnauthorized
	} else {
		status.State = CertManagerStateDegraded
	}
	return status
}

func (c *Client) checkCertManagerAccessContext(ctx context.Context) error {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return err
	}
	for _, resource := range []struct {
		gvr        schema.GroupVersionResource
		namespaced bool
	}{{certGVR, true}, {issuerGVR, true}, {clusterIssuerGVR, false}, {certificateRequestGVR, true}, {orderGVR, true}, {challengeGVR, true}} {
		if resource.namespaced {
			_, err = dynamicClient.Resource(resource.gvr).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
		} else {
			_, err = dynamicClient.Resource(resource.gvr).List(ctx, metav1.ListOptions{Limit: 1})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func deploymentReady(ctx context.Context, c *Client, name string) bool {
	deployment, err := c.Clientset.AppsV1().Deployments(certManagerNamespace).Get(ctx, name, metav1.GetOptions{})
	return err == nil && deployment.Status.AvailableReplicas > 0
}

func (c *Client) helmChartInstallerAvailableContext(ctx context.Context) bool {
	exists, err := c.CheckCRDContext(ctx, "helmcharts.helm.cattle.io")
	return err == nil && exists
}

func (c *Client) certManagerHelmChartStateContext(ctx context.Context) (exists, failed bool, detail string) {
	return c.helmChartStateContext(ctx, certManagerHelmName)
}

func (c *Client) helmChartStateContext(ctx context.Context, name string) (exists, failed bool, detail string) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return false, false, ""
	}
	chart, err := dynamicClient.Resource(helmChartGVR).Namespace("kube-system").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, false, ""
	}
	conditions, _, _ := unstructured.NestedSlice(chart.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok || condition["type"] != "Failed" || condition["status"] != "True" {
			continue
		}
		detail, _, _ = unstructured.NestedString(condition, "message")
		if detail == "" {
			detail, _, _ = unstructured.NestedString(condition, "reason")
		}
		if detail == "" {
			detail = "HelmChart 控制器报告安装失败"
		}
		return true, true, detail
	}
	return true, false, ""
}
