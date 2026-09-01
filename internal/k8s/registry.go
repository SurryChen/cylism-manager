package k8s

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"golang.org/x/crypto/bcrypt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ManagedRegistryReconciler owns Kubernetes resource lifecycle operations for
// the platform-managed Registry. It is intentionally independent from HTTP.
type ManagedRegistryReconciler struct{ client *Client }

// ManagedRegistryResourceReconciler contains mutating resource operations.
type ManagedRegistryResourceReconciler interface {
	EnsureDataNode(context.Context, string) error
	EnsureStorageClass(context.Context, string) error
	ResolvePVC(context.Context, *model.ManagedOCIRegistry) error
	EnsureTLSCertificate(context.Context, *model.ManagedOCIRegistry) error
	EnsureResourcesAvailable(context.Context, string, string) error
	Apply(context.Context, *model.ManagedOCIRegistry, string, string) error
	DeleteResources(context.Context, *model.ManagedOCIRegistry)
}

// ManagedRegistryStatusReader contains read-only discovery and status calls.
type ManagedRegistryStatusReader interface {
	Available() bool
	StoragePreflight(context.Context, string) ([]string, error)
	ListReadyDataNodes(context.Context) ([]string, error)
	ListEligiblePVCs(context.Context, string) ([]ManagedRegistryPVCOption, error)
	ListMatchingCertificates(context.Context, string, string) ([]ManagedRegistryCertificateOption, error)
	LegacyHostPath(context.Context, *model.ManagedOCIRegistry) (bool, error)
	ManagedRegistryStatus(context.Context, *model.ManagedOCIRegistry) (string, string, string)
}

var _ ManagedRegistryResourceReconciler = (*ManagedRegistryReconciler)(nil)
var _ ManagedRegistryStatusReader = (*ManagedRegistryReconciler)(nil)

const managedRegistryResourceName = "cylism-oci-registry"

// ManagedRegistryPVCOption is a PVC that can be mounted by the single-replica
// platform Registry. It deliberately describes only existing claims.
type ManagedRegistryPVCOption struct {
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace"`
	Storage          string   `json:"storage"`
	StorageClassName string   `json:"storage_class_name"`
	Phase            string   `json:"phase"`
	AccessModes      []string `json:"access_modes"`
	BoundNode        string   `json:"bound_node,omitempty"`
}

// ManagedRegistryCertificateOption is a safe certificate summary for the
// Registry form. The TLS Secret name remains internal to reconciliation.
type ManagedRegistryCertificateOption struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Domains     []string `json:"domains"`
	ExpiryDate  string   `json:"expiry_date,omitempty"`
	RenewalTime string   `json:"renewal_time,omitempty"`
}

func NewManagedRegistryReconciler(client *Client) *ManagedRegistryReconciler {
	return &ManagedRegistryReconciler{client: client}
}

func (r *ManagedRegistryReconciler) Available() bool {
	return r != nil && r.client != nil && r.client.Clientset != nil
}

// StoragePreflight checks the fixed local-path storage contract and returns
// all Ready nodes that can be selected for an unbound Registry PVC.
func (r *ManagedRegistryReconciler) StoragePreflight(ctx context.Context, storageClassName string) ([]string, error) {
	if err := r.EnsureStorageClass(ctx, storageClassName); err != nil {
		return nil, err
	}
	return r.ListReadyDataNodes(ctx)
}

func (r *ManagedRegistryReconciler) ListReadyDataNodes(ctx context.Context) ([]string, error) {
	if !r.Available() {
		return nil, errors.New("Kubernetes 集群未连接")
	}
	nodes, err := r.client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	ready := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				ready = append(ready, node.Name)
				break
			}
		}
	}
	sort.Strings(ready)
	return ready, nil
}

func (r *ManagedRegistryReconciler) EnsureDataNode(ctx context.Context, name string) error {
	if !r.Available() {
		return errors.New("Kubernetes 集群未连接")
	}
	node, err := r.client.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("数据节点 %q 不存在", name)
	}
	if err != nil {
		return fmt.Errorf("读取数据节点失败: %w", err)
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return nil
		}
	}
	return fmt.Errorf("数据节点 %q 未处于 Ready 状态", name)
}

func (r *ManagedRegistryReconciler) EnsureStorageClass(ctx context.Context, storageClassName string) error {
	if !r.Available() {
		return errors.New("Kubernetes 集群未连接")
	}
	storageClass, err := r.client.Clientset.StorageV1().StorageClasses().Get(ctx, storageClassName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("StorageClass %q 不存在", storageClassName)
	}
	if err != nil {
		return fmt.Errorf("读取 StorageClass 失败: %w", err)
	}
	if storageClass.VolumeBindingMode == nil || *storageClass.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
		return fmt.Errorf("StorageClass %q 必须使用 WaitForFirstConsumer", storageClassName)
	}
	return nil
}

// ResolvePVC verifies the Registry-specific storage contract and fills the
// immutable storage metadata from the selected existing PVC.
func (r *ManagedRegistryReconciler) ResolvePVC(ctx context.Context, registry *model.ManagedOCIRegistry) error {
	if !r.Available() {
		return errors.New("Kubernetes 集群未连接")
	}
	claim, err := r.client.Clientset.CoreV1().PersistentVolumeClaims(registry.Namespace).Get(ctx, registry.PVCName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("PVC %q 不存在于命名空间 %q", registry.PVCName, registry.Namespace)
	}
	if err != nil {
		return fmt.Errorf("读取 PVC %q 失败: %w", registry.PVCName, err)
	}
	info, err := r.client.pvcInfo(claim)
	if err != nil {
		return fmt.Errorf("读取 PVC %q 失败: %w", registry.PVCName, err)
	}
	if err := ValidateManagedRegistryPVC(info, registry.Namespace); err != nil {
		return fmt.Errorf("PVC %q 不可用于制品库: %w", registry.PVCName, err)
	}
	if err := r.ensurePVCUnused(ctx, registry, info.Name); err != nil {
		return err
	}
	if info.BoundNode != "" {
		if registry.DataNode != "" && registry.DataNode != info.BoundNode {
			return fmt.Errorf("PVC %q 已绑定到节点 %q，不能选择节点 %q", registry.PVCName, info.BoundNode, registry.DataNode)
		}
		registry.DataNode = info.BoundNode
	}
	if strings.TrimSpace(registry.DataNode) == "" {
		return errors.New("待绑定 PVC 必须选择数据节点")
	}
	registry.StorageClassName, registry.StorageSize = info.StorageClassName, info.Storage
	return nil
}

func ValidateManagedRegistryPVC(claim PersistentVolumeClaimInfo, namespace string) error {
	if claim.Namespace != namespace {
		return fmt.Errorf("必须位于命名空间 %q", namespace)
	}
	if claim.StorageClassName != "local-path" {
		return errors.New("仅支持 local-path StorageClass")
	}
	if !claim.WaitForFirstConsumer {
		return errors.New("StorageClass 必须使用 WaitForFirstConsumer")
	}
	if claim.Phase != string(corev1.ClaimPending) && claim.Phase != string(corev1.ClaimBound) {
		return fmt.Errorf("PVC 状态必须为 Pending 或 Bound，当前为 %q", claim.Phase)
	}
	if !containsAccessMode(claim.AccessModes, string(corev1.ReadWriteOnce)) {
		return errors.New("必须支持 ReadWriteOnce 访问模式")
	}
	quantity, err := resource.ParseQuantity(claim.Storage)
	if claim.Storage == "" || err != nil || quantity.Sign() <= 0 {
		return errors.New("存储容量无效")
	}
	return nil
}

func (r *ManagedRegistryReconciler) ListEligiblePVCs(ctx context.Context, namespace string) ([]ManagedRegistryPVCOption, error) {
	if !r.Available() {
		return nil, errors.New("Kubernetes 集群未连接")
	}
	claims, err := r.client.ListPVCs(namespace)
	if err != nil {
		return nil, err
	}
	options := make([]ManagedRegistryPVCOption, 0, len(claims))
	for _, claim := range claims {
		if ValidateManagedRegistryPVC(claim, namespace) != nil {
			continue
		}
		inUse, err := r.pvcInUseExceptRegistry(ctx, namespace, claim.Name, managedRegistryResourceName)
		if err != nil {
			return nil, err
		}
		if inUse == "" {
			options = append(options, ManagedRegistryPVCOption{Name: claim.Name, Namespace: claim.Namespace, Storage: claim.Storage, StorageClassName: claim.StorageClassName, Phase: claim.Phase, AccessModes: claim.AccessModes, BoundNode: claim.BoundNode})
		}
	}
	return options, nil
}

func (r *ManagedRegistryReconciler) ensurePVCUnused(ctx context.Context, registry *model.ManagedOCIRegistry, claimName string) error {
	inUse, err := r.pvcInUseExceptRegistry(ctx, registry.Namespace, claimName, registry.ResourceName)
	if err != nil {
		return err
	}
	if inUse != "" {
		return fmt.Errorf("PVC %q 已被 %s 引用", claimName, inUse)
	}
	return nil
}

func (r *ManagedRegistryReconciler) pvcInUse(ctx context.Context, namespace, claimName string) (bool, error) {
	workload, err := r.pvcInUseExceptRegistry(ctx, namespace, claimName, "")
	return workload != "", err
}

func (r *ManagedRegistryReconciler) pvcInUseExceptRegistry(ctx context.Context, namespace, claimName, ownedRegistryName string) (string, error) {
	deployments, err := r.client.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("读取 PVC 工作负载引用失败: %w", err)
	}
	for index := range deployments.Items {
		deployment := &deployments.Items[index]
		if deployment.Name == ownedRegistryName && deployment.Labels["cylism.io/managed-registry"] == "true" {
			continue
		}
		if DeploymentReferencesPVC(deployment, claimName) {
			return "Deployment " + deployment.Name, nil
		}
	}
	statefulSets, err := r.client.Clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("读取 PVC 工作负载引用失败: %w", err)
	}
	for index := range statefulSets.Items {
		if StatefulSetReferencesPVC(&statefulSets.Items[index], claimName) {
			return "StatefulSet " + statefulSets.Items[index].Name, nil
		}
	}
	return "", nil
}

func (r *ManagedRegistryReconciler) ListMatchingCertificates(ctx context.Context, namespace, host string) ([]ManagedRegistryCertificateOption, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("Kubernetes 集群未连接")
	}
	certificates, err := r.client.ListCertificates()
	if err != nil {
		return nil, err
	}
	options := make([]ManagedRegistryCertificateOption, 0)
	for _, certificate := range certificates {
		if certificate.Namespace != namespace || certificate.Status != "Ready" || certificate.SecretName == "" || (host != "" && !certificateDomainsCoverHostname(certificate.Domains, host)) {
			continue
		}
		options = append(options, ManagedRegistryCertificateOption{Name: certificate.Name, Namespace: certificate.Namespace, Domains: certificate.Domains, ExpiryDate: certificate.ExpiryDate, RenewalTime: certificate.RenewalTime})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Name < options[j].Name })
	return options, nil
}

// EnsureTLSCertificate resolves a ready Certificate and validates its Secret
// before the Ingress is reconciled. Insecure HTTP intentionally clears TLS.
func (r *ManagedRegistryReconciler) EnsureTLSCertificate(ctx context.Context, registry *model.ManagedOCIRegistry) error {
	if registry.InsecureHTTP {
		registry.CertificateName, registry.TLSSecretName = "", ""
		return nil
	}
	if r == nil || r.client == nil || r.client.Clientset == nil {
		return errors.New("Kubernetes 集群未连接")
	}
	certificate, err := r.client.GetCertificate(registry.Namespace, registry.CertificateName)
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("证书 %q 不存在或不属于命名空间 %q", registry.CertificateName, registry.Namespace)
	}
	if err != nil {
		return fmt.Errorf("读取证书失败: %w", err)
	}
	if certificate.Status != "Ready" {
		return fmt.Errorf("证书 %q 尚未就绪", registry.CertificateName)
	}
	host := registryEndpointHostname(registry.Endpoint)
	if host == "" {
		return errors.New("制品库地址无效")
	}
	if !certificateDomainsCoverHostname(certificate.Domains, host) {
		return fmt.Errorf("证书 %q 未覆盖访问域名 %q", registry.CertificateName, host)
	}
	if strings.TrimSpace(certificate.SecretName) == "" {
		return fmt.Errorf("证书 %q 未配置 TLS Secret", registry.CertificateName)
	}
	registry.TLSSecretName = certificate.SecretName
	secret, err := r.client.Clientset.CoreV1().Secrets(registry.Namespace).Get(ctx, registry.TLSSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("TLS Secret %q 不存在", registry.TLSSecretName)
	}
	if err != nil {
		return fmt.Errorf("读取 TLS Secret 失败: %w", err)
	}
	if secret.Type != corev1.SecretTypeTLS || len(secret.Data[corev1.TLSCertKey]) == 0 || len(secret.Data[corev1.TLSPrivateKeyKey]) == 0 {
		return fmt.Errorf("TLS Secret %q 必须包含 tls.crt 和 tls.key", registry.TLSSecretName)
	}
	return nil
}

func containsAccessMode(modes []string, expected string) bool {
	for _, mode := range modes {
		if mode == expected {
			return true
		}
	}
	return false
}

func registryEndpointHostname(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(endpoint); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(endpoint)
}

func certificateDomainsCoverHostname(domains []string, hostname string) bool {
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		hostname = strings.ToLower(strings.TrimSpace(hostname))
		if domain == hostname {
			return true
		}
		if strings.HasPrefix(domain, "*.") {
			suffix := strings.TrimPrefix(domain, "*")
			if strings.HasSuffix(hostname, suffix) {
				prefix := strings.TrimSuffix(hostname, suffix)
				if prefix != "" && !strings.Contains(prefix, ".") {
					return true
				}
			}
		}
	}
	return false
}

func (r *ManagedRegistryReconciler) DeleteResources(ctx context.Context, registry *model.ManagedOCIRegistry) {
	_ = r.client.Clientset.NetworkingV1().Ingresses(registry.Namespace).Delete(ctx, registry.ResourceName, metav1.DeleteOptions{})
	_ = r.client.Clientset.CoreV1().Services(registry.Namespace).Delete(ctx, registry.ResourceName, metav1.DeleteOptions{})
	_ = r.client.Clientset.AppsV1().Deployments(registry.Namespace).Delete(ctx, registry.ResourceName, metav1.DeleteOptions{})
	_ = r.client.Clientset.CoreV1().Secrets(registry.Namespace).Delete(ctx, registry.ResourceName+"-auth", metav1.DeleteOptions{})
}

func (r *ManagedRegistryReconciler) EnsureResourcesAvailable(ctx context.Context, namespace, name string) error {
	checks := []struct {
		kind string
		get  func() (metav1.Object, error)
	}{
		{"Deployment", func() (metav1.Object, error) {
			return r.client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		}},
		{"Service", func() (metav1.Object, error) {
			return r.client.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		}},
		{"Ingress", func() (metav1.Object, error) {
			return r.client.Clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		}},
		{"认证 Secret", func() (metav1.Object, error) {
			return r.client.Clientset.CoreV1().Secrets(namespace).Get(ctx, name+"-auth", metav1.GetOptions{})
		}},
	}
	for _, check := range checks {
		object, err := check.get()
		if err == nil && object.GetLabels()["cylism.io/managed-registry"] != "true" {
			return errors.New("同名" + check.kind + "不属于 Cylism Manager，不能接管")
		}
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *ManagedRegistryReconciler) Apply(ctx context.Context, registry *model.ManagedOCIRegistry, host, password string) error {
	if err := r.ensureNamespace(ctx, registry.Namespace); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("生成制品库认证凭据失败")
	}
	labels := ManagedRegistryLabels(registry)
	if err := r.upsertSecret(ctx, ManagedRegistryAuthSecret(registry, labels, hash)); err != nil {
		return err
	}
	if err := r.upsertDeployment(ctx, ManagedRegistryDeployment(registry, labels)); err != nil {
		return err
	}
	if err := r.upsertService(ctx, ManagedRegistryService(registry, labels)); err != nil {
		return err
	}
	return r.upsertIngress(ctx, ManagedRegistryIngress(registry, host, labels))
}

func (r *ManagedRegistryReconciler) ensureNamespace(ctx context.Context, namespace string) error {
	_, err := r.client.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil || !apierrors.IsNotFound(err) {
		return err
	}
	_, err = r.client.Clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "cylism-manager"}}}, metav1.CreateOptions{})
	return err
}

func (r *ManagedRegistryReconciler) upsertSecret(ctx context.Context, desired *corev1.Secret) error {
	resources := r.client.Clientset.CoreV1().Secrets(desired.Namespace)
	current, err := resources.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resources.Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if current.Labels["cylism.io/managed-registry"] != "true" {
		return errors.New("同名认证 Secret 不属于 Cylism Manager")
	}
	desired.ResourceVersion = current.ResourceVersion
	_, err = resources.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (r *ManagedRegistryReconciler) upsertDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	resources := r.client.Clientset.AppsV1().Deployments(desired.Namespace)
	current, err := resources.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resources.Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if current.Labels["cylism.io/managed-registry"] != "true" {
		return errors.New("同名 Deployment 不属于 Cylism Manager")
	}
	desired.ResourceVersion = current.ResourceVersion
	_, err = resources.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (r *ManagedRegistryReconciler) upsertService(ctx context.Context, desired *corev1.Service) error {
	resources := r.client.Clientset.CoreV1().Services(desired.Namespace)
	current, err := resources.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resources.Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if current.Labels["cylism.io/managed-registry"] != "true" {
		return errors.New("同名 Service 不属于 Cylism Manager")
	}
	desired.ResourceVersion, desired.Spec.ClusterIP = current.ResourceVersion, current.Spec.ClusterIP
	_, err = resources.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (r *ManagedRegistryReconciler) upsertIngress(ctx context.Context, desired *networkingv1.Ingress) error {
	resources := r.client.Clientset.NetworkingV1().Ingresses(desired.Namespace)
	current, err := resources.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resources.Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if current.Labels["cylism.io/managed-registry"] != "true" {
		return errors.New("同名 Ingress 不属于 Cylism Manager")
	}
	desired.ResourceVersion = current.ResourceVersion
	_, err = resources.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (r *ManagedRegistryReconciler) LegacyHostPath(ctx context.Context, registry *model.ManagedOCIRegistry) (bool, error) {
	deployment, err := r.client.Clientset.AppsV1().Deployments(registry.Namespace).Get(ctx, registry.ResourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "storage" && volume.HostPath != nil {
			return true, nil
		}
	}
	return false, nil
}

func EndpointReady(ctx context.Context, endpoint string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(endpoint, "/")+"/v2/", nil)
	if err != nil {
		return false
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK || response.StatusCode == http.StatusUnauthorized
}

// ManagedRegistryStatus reads the cluster state without mutating persistence.
func (r *ManagedRegistryReconciler) ManagedRegistryStatus(ctx context.Context, registry *model.ManagedOCIRegistry) (phase, status, detail string) {
	legacy, err := r.LegacyHostPath(ctx, registry)
	if legacy {
		return "", "migration_required", "现有 Registry 仍使用旧 hostPath 存储，需要迁移到 PVC 后才能继续管理"
	}
	if err != nil {
		return "Unknown", "degraded", err.Error()
	}
	pvc, err := r.client.Clientset.CoreV1().PersistentVolumeClaims(registry.Namespace).Get(ctx, registry.PVCName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "Missing", "degraded", "Registry PVC 不存在"
	}
	if err != nil {
		return "Unknown", "degraded", err.Error()
	}
	phase = string(pvc.Status.Phase)
	if pvc.Status.Phase != corev1.ClaimBound {
		return phase, "pending", "Registry PVC 等待绑定到所选节点"
	}
	deployment, err := r.client.Clientset.AppsV1().Deployments(registry.Namespace).Get(ctx, registry.ResourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return phase, "degraded", "Registry Deployment 不存在"
	}
	if err != nil {
		return phase, "degraded", err.Error()
	}
	if deployment.Status.ReadyReplicas == 0 {
		return phase, "degraded", "Registry Deployment 尚未就绪"
	}
	scheme := "https"
	if registry.InsecureHTTP {
		scheme = "http"
	}
	if !EndpointReady(ctx, scheme+"://"+registry.Endpoint) {
		return phase, "degraded", "Registry 入口 /v2/ 未就绪"
	}
	return phase, "ready", ""
}

// ManagedRegistryLabels identifies resources owned by the platform Registry.
func ManagedRegistryLabels(registry *model.ManagedOCIRegistry) map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "app.kubernetes.io/name": registry.ResourceName, "cylism.io/managed-registry": "true"}
}

func ManagedRegistryAuthSecret(registry *model.ManagedOCIRegistry, labels map[string]string, hash []byte) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: registry.ResourceName + "-auth", Namespace: registry.Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"htpasswd": []byte(registry.PullUsername + ":" + string(hash) + "\n")}}
}

func ManagedRegistryDeployment(registry *model.ManagedOCIRegistry, labels map[string]string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: registry.ResourceName, Namespace: registry.Namespace, Labels: labels}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: registry.DataNode}, Volumes: []corev1.Volume{{Name: "storage", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: registry.PVCName}}}, {Name: "auth", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: registry.ResourceName + "-auth"}}}}, Containers: []corev1.Container{{Name: "registry", Image: registry.RegistryImage, Ports: []corev1.ContainerPort{{Name: "registry", ContainerPort: 5000}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(registry.CPURequest), corev1.ResourceMemory: resource.MustParse(registry.MemoryRequest)}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(registry.CPULimit), corev1.ResourceMemory: resource.MustParse(registry.MemoryLimit)}}, Env: []corev1.EnvVar{{Name: "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", Value: "/var/lib/registry"}, {Name: "REGISTRY_AUTH", Value: "htpasswd"}, {Name: "REGISTRY_AUTH_HTPASSWD_REALM", Value: "Cylism Registry"}, {Name: "REGISTRY_AUTH_HTPASSWD_PATH", Value: "/auth/htpasswd"}}, VolumeMounts: []corev1.VolumeMount{{Name: "storage", MountPath: "/var/lib/registry"}, {Name: "auth", MountPath: "/auth", ReadOnly: true}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(5000)}}, InitialDelaySeconds: 3, PeriodSeconds: 5}}}}}}}
}

func ManagedRegistryService(registry *model.ManagedOCIRegistry, labels map[string]string) *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: registry.ResourceName, Namespace: registry.Namespace, Labels: labels}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: labels, Ports: []corev1.ServicePort{{Name: "registry", Port: 5000, TargetPort: intstr.FromInt(5000)}}}}
}

func ManagedRegistryIngress(registry *model.ManagedOCIRegistry, host string, labels map[string]string) *networkingv1.Ingress {
	pathType, ingressClass := networkingv1.PathTypePrefix, "traefik"
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: registry.ResourceName, Namespace: registry.Namespace, Labels: labels}, Spec: networkingv1.IngressSpec{IngressClassName: &ingressClass, Rules: []networkingv1.IngressRule{{Host: host, IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &pathType, Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: registry.ResourceName, Port: networkingv1.ServiceBackendPort{Number: 5000}}}}}}}}}}}
	if !registry.InsecureHTTP {
		ingress.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{host}, SecretName: registry.TLSSecretName}}
	}
	return ingress
}
