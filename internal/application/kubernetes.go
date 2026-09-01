package application

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type KubernetesApplier struct {
	resources        ApplicationResourceApplier
	preflight        ApplicationPreflightReader
	workloads        ApplicationWorkloadController
	endpoints        ApplicationEndpointReader
	diagnostics      ReleaseDiagnosticsReader
	ReadinessTimeout time.Duration
}

// ApplicationPreflightReader reads only the cluster state needed before a
// release changes resources.
type ApplicationPreflightReader interface {
	GetNamespace(context.Context, string) (*corev1.Namespace, error)
	GetSecret(context.Context, string, string) (*corev1.Secret, error)
	GetConfigMap(context.Context, string, string) (*corev1.ConfigMap, error)
	GetNode(context.Context, string) (*corev1.Node, error)
	DetectIngressController(context.Context) (*k8sclient.IngressControllerStatus, error)
	CheckCRD(context.Context, string) (bool, error)
	GetManagedPVC(context.Context, string, string, uint) (*k8sclient.PersistentVolumeClaimInfo, error)
}

// ApplicationWorkloadController owns Deployment and StatefulSet mutations.
type ApplicationWorkloadController interface {
	GetDeployment(context.Context, string, string) (*appsv1.Deployment, error)
	UpdateDeployment(context.Context, *appsv1.Deployment) (*appsv1.Deployment, error)
	GetStatefulSet(context.Context, string, string) (*appsv1.StatefulSet, error)
	UpdateStatefulSet(context.Context, *appsv1.StatefulSet) (*appsv1.StatefulSet, error)
}

// ApplicationEndpointReader owns the existing Ingress read/delete boundary.
type ApplicationEndpointReader interface {
	DeleteIngress(context.Context, string, string) error
	GetIngress(context.Context, string, string) (*networkingv1.Ingress, error)
}

// ReleaseDiagnosticsReader is the bounded runtime and certificate read path.
type ReleaseDiagnosticsReader interface {
	ListPods(context.Context, string, string) ([]corev1.Pod, error)
	ListEvents(context.Context, string) ([]corev1.Event, error)
	CertificateReady(context.Context, string, string) (bool, error)
}

// ApplicationResourceApplier owns the Kubernetes resource mutation boundary
// used by release rendering. Keeping this interface separate from the client
// also lets release orchestration tests use a fake without a full cluster
// client.
type ApplicationResourceApplier interface {
	ApplyConfigMap(context.Context, *corev1.ConfigMap) error
	ApplySecret(context.Context, *corev1.Secret) error
	ApplyDeployment(context.Context, *appsv1.Deployment) error
	ApplyStatefulSet(context.Context, *appsv1.StatefulSet) error
	ApplyService(context.Context, *corev1.Service) error
	ApplyIngress(context.Context, *networkingv1.Ingress) error
	ApplyCertificate(context.Context, *unstructured.Unstructured) error
}

func NewKubernetesApplier(client *k8sclient.Client) *KubernetesApplier {
	return &KubernetesApplier{
		resources:        k8sclient.NewApplicationResourceApplier(client),
		preflight:        k8sclient.NewApplicationPreflightAdapter(client),
		workloads:        k8sclient.NewApplicationWorkloadControllerAdapter(client),
		endpoints:        k8sclient.NewApplicationEndpointAdapter(client),
		diagnostics:      k8sclient.NewReleaseDiagnosticsAdapter(client),
		ReadinessTimeout: 2 * time.Minute,
	}
}

// VerifyImage confirms that the selected repository and tag can be resolved
// before Kubernetes resources are changed. Node-level pulling is validated by
// WaitReady after the Deployment is applied.
func (a *KubernetesApplier) VerifyImage(ctx context.Context, spec ReleaseSpec) error {
	ref, err := imageVerificationReference(spec)
	if err != nil {
		return fmt.Errorf("镜像地址无效: %w", err)
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	options := []remote.Option{remote.WithContext(verifyCtx), remote.WithAuth(authn.Anonymous)}
	if spec.ImageVerificationEndpoint != "" {
		if spec.ImageVerificationUsername != "" {
			options[1] = remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: spec.ImageVerificationUsername, Password: spec.ImageVerificationCredential}))
		}
		if spec.ImageVerificationInsecureSkipVerify {
			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			options = append(options, remote.WithTransport(transport))
		}
	} else {
		switch strings.TrimSpace(spec.RegistryAuthType) {
		case "", "anonymous":
		case "token":
			options[1] = remote.WithAuth(authn.FromConfig(authn.AuthConfig{RegistryToken: spec.RegistryCredential}))
		default:
			options[1] = remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: spec.RegistryUsername, Password: spec.RegistryCredential}))
		}
	}
	if _, err := remote.Head(ref, options...); err != nil {
		return fmt.Errorf("镜像版本 %q 不存在或不可访问: %s", spec.Image, redactReleaseDiagnostic(err.Error(), spec))
	}
	return nil
}

func imageVerificationReference(spec ReleaseSpec) (name.Reference, error) {
	source, err := name.ParseReference(strings.TrimSpace(spec.Image))
	if err != nil || strings.TrimSpace(spec.ImageVerificationEndpoint) == "" {
		return source, err
	}
	target, err := url.ParseRequestURI(spec.ImageVerificationEndpoint)
	if err != nil || target.Host == "" || (target.Scheme != "https" && target.Scheme != "http") || (target.Path != "" && target.Path != "/") {
		return nil, fmt.Errorf("镜像代理地址无效")
	}
	options := []name.Option{}
	if target.Scheme == "http" {
		options = append(options, name.Insecure)
	}
	repository, err := name.NewRepository(target.Host+"/"+source.Context().RepositoryStr(), options...)
	if err != nil {
		return nil, err
	}
	switch reference := source.(type) {
	case name.Tag:
		return repository.Tag(reference.TagStr()), nil
	case name.Digest:
		return repository.Digest(reference.DigestStr()), nil
	default:
		return nil, fmt.Errorf("镜像引用格式无效")
	}
}

func (a *KubernetesApplier) Preflight(ctx context.Context, application ApplicationContext, spec ReleaseSpec) error {
	if a.preflight == nil || a.resources == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	namespace, err := a.preflight.GetNamespace(ctx, application.Namespace)
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("环境命名空间 %q 不存在，请在环境页面创建或同步命名空间", application.Namespace)
	}
	if err != nil {
		return fmt.Errorf("检查环境命名空间: %w", err)
	}
	if namespace.Status.Phase != corev1.NamespaceActive {
		return fmt.Errorf("环境命名空间 %q 未就绪", application.Namespace)
	}
	if err := a.ValidatePersistentVolumeClaims(ctx, application, spec); err != nil {
		return err
	}
	if err := a.ValidateFileMountSources(ctx, application, spec); err != nil {
		return err
	}
	if spec.Endpoint.Exposure != ExposurePublic {
		return nil
	}
	status, err := a.preflight.DetectIngressController(ctx)
	if err != nil || status == nil || !status.Running {
		return fmt.Errorf("Ingress Controller 未就绪")
	}
	if spec.Endpoint.TLSEnabled {
		ok, err := a.preflight.CheckCRD(ctx, "certificates.cert-manager.io")
		if err != nil || !ok {
			return fmt.Errorf("cert-manager Certificate CRD 不可用")
		}
	}
	return nil
}

// ValidateFileMountSources ensures every projected object and requested key
// exists before the workload is applied. All references are scoped to the
// application's namespace by construction.
func (a *KubernetesApplier) ValidateFileMountSources(ctx context.Context, application ApplicationContext, spec ReleaseSpec) error {
	config := enabledStringMap(spec.Config, spec.ConfigDisabled)
	secrets := enabledStringMap(spec.Secrets, spec.SecretsDisabled)
	for _, fileMount := range spec.FileMounts {
		switch fileMount.SourceType {
		case FileMountSourceApplicationConfig:
			if _, ok := config[fileMount.Key]; !ok {
				return fmt.Errorf("当前应用 ConfigMap 不包含键 %q", fileMount.Key)
			}
		case FileMountSourceApplicationSecret:
			if _, ok := secrets[fileMount.Key]; !ok {
				return fmt.Errorf("当前应用 Secret 不包含键 %q", fileMount.Key)
			}
		case FileMountSourceSecret:
			secret, err := a.preflight.GetSecret(ctx, application.Namespace, fileMount.SourceName)
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("文件挂载 Secret %q 不存在", fileMount.SourceName)
			}
			if err != nil {
				return fmt.Errorf("读取文件挂载 Secret %q: %w", fileMount.SourceName, err)
			}
			if _, ok := secret.Data[fileMount.Key]; !ok {
				return fmt.Errorf("文件挂载 Secret %q 不包含键 %q", fileMount.SourceName, fileMount.Key)
			}
		case FileMountSourceConfigMap:
			configMap, err := a.preflight.GetConfigMap(ctx, application.Namespace, fileMount.SourceName)
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("文件挂载 ConfigMap %q 不存在", fileMount.SourceName)
			}
			if err != nil {
				return fmt.Errorf("读取文件挂载 ConfigMap %q: %w", fileMount.SourceName, err)
			}
			if _, inData := configMap.Data[fileMount.Key]; !inData {
				if _, inBinaryData := configMap.BinaryData[fileMount.Key]; !inBinaryData {
					return fmt.Errorf("文件挂载 ConfigMap %q 不包含键 %q", fileMount.SourceName, fileMount.Key)
				}
			}
		default:
			return fmt.Errorf("不支持的文件挂载来源类型 %q", fileMount.SourceType)
		}
	}
	return nil
}

// ValidatePersistentVolumeClaims validates platform-owned claims before a
// template is saved or a release is applied. WFFC claims are allowed to remain
// Pending when an explicit node is selected because the first Pod performs the
// binding.
func (a *KubernetesApplier) ValidatePersistentVolumeClaims(ctx context.Context, application ApplicationContext, spec ReleaseSpec) error {
	if len(spec.Volumes) == 0 {
		return nil
	}
	if spec.Replicas != 1 {
		return fmt.Errorf("ReadWriteOnce PVC 仅支持单副本应用")
	}
	if spec.NodeName != "" {
		if _, err := a.preflight.GetNode(ctx, spec.NodeName); err != nil {
			return fmt.Errorf("检查部署节点 %q: %w", spec.NodeName, err)
		}
	}
	for _, volume := range spec.Volumes {
		claim, err := a.preflight.GetManagedPVC(ctx, application.Namespace, volume.ClaimName, application.EnvironmentID)
		if err != nil {
			return err
		}
		switch claim.Phase {
		case string(corev1.ClaimBound):
			if claim.BoundNode != "" && claim.BoundNode != spec.NodeName {
				return fmt.Errorf("PVC %q 已绑定节点 %q，部署节点必须保持一致", volume.ClaimName, claim.BoundNode)
			}
		case string(corev1.ClaimPending):
			if !claim.WaitForFirstConsumer || spec.NodeName == "" {
				return fmt.Errorf("PVC %q 尚未就绪；本地卷首次挂载前请选择部署节点", volume.ClaimName)
			}
		default:
			return fmt.Errorf("PVC %q 当前状态为 %s，不能挂载", volume.ClaimName, claim.Phase)
		}
	}
	return nil
}

func (a *KubernetesApplier) Apply(ctx context.Context, resources *RenderedResources) error {
	if a.resources == nil {
		return fmt.Errorf("Kubernetes 资源应用器未初始化")
	}
	if resources.ImagePullSecret != nil {
		if err := a.resources.ApplySecret(ctx, resources.ImagePullSecret); err != nil {
			return err
		}
	}
	if resources.ConfigMap != nil {
		if err := a.resources.ApplyConfigMap(ctx, resources.ConfigMap); err != nil {
			return err
		}
	}
	if resources.Secret != nil {
		if err := a.resources.ApplySecret(ctx, resources.Secret); err != nil {
			return err
		}
	}
	if resources.Deployment != nil {
		if err := a.resources.ApplyDeployment(ctx, resources.Deployment); err != nil {
			return err
		}
	}
	if resources.StatefulSet != nil {
		if err := a.resources.ApplyStatefulSet(ctx, resources.StatefulSet); err != nil {
			return err
		}
	}
	if err := a.resources.ApplyService(ctx, resources.Service); err != nil {
		return err
	}
	if resources.Certificate != nil {
		if err := a.resources.ApplyCertificate(ctx, resources.Certificate); err != nil {
			return err
		}
	}
	if resources.Ingress != nil {
		if err := a.resources.ApplyIngress(ctx, resources.Ingress); err != nil {
			return err
		}
	}
	return nil
}

// SyncEndpoint updates only the application Ingress. Domain ownership is
// application-level state, so it must not wait for a version release.
func (a *KubernetesApplier) SyncEndpoint(ctx context.Context, application ApplicationContext, endpoint EndpointSpec, servicePort int32) error {
	return a.SyncApplicationEndpoints(ctx, application, []model.ApplicationEndpoint{{
		Domain:        endpoint.Domain,
		Path:          endpoint.Path,
		Exposure:      endpoint.Exposure,
		TLSEnabled:    endpoint.TLSEnabled,
		TLSSecretName: endpoint.ManagedTLSSecretName,
	}}, servicePort)
}

// SyncApplicationEndpoints reconciles all public application endpoints into one managed Ingress.
func (a *KubernetesApplier) SyncApplicationEndpoints(ctx context.Context, application ApplicationContext, endpoints []model.ApplicationEndpoint, servicePort int32) error {
	if a.endpoints == nil || a.resources == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	publicEndpoints := make([]model.ApplicationEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		// A domain binding can be metadata-only for non-HTTP protocols. Keep it
		// visible to discovery while excluding it from the managed Ingress.
		if endpoint.IngressMode != "metadata" && (endpoint.Exposure == "" || endpoint.Exposure == ExposurePublic) {
			publicEndpoints = append(publicEndpoints, endpoint)
		}
	}
	if len(publicEndpoints) == 0 {
		return a.RemoveEndpoint(ctx, application)
	}
	return a.resources.ApplyIngress(ctx, applicationEndpointsIngress(application, publicEndpoints, servicePort))
}

func (a *KubernetesApplier) RemoveEndpoint(ctx context.Context, application ApplicationContext) error {
	if a.endpoints == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	ingress, err := a.endpoints.GetIngress(ctx, application.Namespace, application.ApplicationName)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(ingress.Labels); err != nil {
		return err
	}
	return a.endpoints.DeleteIngress(ctx, application.Namespace, application.ApplicationName)
}

func (a *KubernetesApplier) WaitReady(ctx context.Context, application ApplicationContext, spec ReleaseSpec) error {
	timeout := a.ReadinessTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastDiagnostic := ""
	for {
		ready, err := a.workloadReady(deadline, application, spec.Replicas)
		if err != nil {
			return err
		}
		if diagnostic, terminal := a.deploymentFailureDiagnostic(deadline, application, spec); diagnostic != "" {
			if terminal {
				return fmt.Errorf("工作负载启动失败: %s", diagnostic)
			}
			lastDiagnostic = diagnostic
		}
		if ready {
			if !spec.Endpoint.TLSEnabled {
				return nil
			}
			ready, err := a.certificateReady(deadline, application, spec.Endpoint)
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
		}
		select {
		case <-deadline.Done():
			if lastDiagnostic != "" {
				return fmt.Errorf("等待工作负载就绪超时: %s", lastDiagnostic)
			}
			return fmt.Errorf("等待工作负载就绪超时")
		case <-ticker.C:
		}
	}
}

func (a *KubernetesApplier) workloadReady(ctx context.Context, application ApplicationContext, replicas int32) (bool, error) {
	if application.WorkloadKind == WorkloadKindStatefulSet {
		statefulSet, err := a.workloads.GetStatefulSet(ctx, application.Namespace, application.ApplicationName)
		if err != nil {
			return false, fmt.Errorf("读取 StatefulSet 就绪状态: %w", err)
		}
		return statefulSet.Status.ObservedGeneration >= statefulSet.Generation && statefulSet.Status.ReadyReplicas >= replicas && statefulSet.Status.CurrentReplicas >= replicas, nil
	}
	deployment, err := a.workloads.GetDeployment(ctx, application.Namespace, application.ApplicationName)
	if err != nil {
		return false, fmt.Errorf("读取 Deployment 就绪状态: %w", err)
	}
	return deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.UpdatedReplicas >= replicas && deployment.Status.AvailableReplicas >= replicas && deployment.Status.UnavailableReplicas == 0, nil
}

// MigrateWorkloadKind replaces one managed controller with the other while
// retaining the application name, Service and directly referenced PVCs.
func (a *KubernetesApplier) MigrateWorkloadKind(ctx context.Context, application ApplicationContext, spec ReleaseSpec, targetKind string) error {
	if a.workloads == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	currentKind := application.WorkloadKind
	if currentKind == "" {
		currentKind = WorkloadKindDeployment
	}
	if currentKind == targetKind {
		return nil
	}
	if targetKind != WorkloadKindDeployment && targetKind != WorkloadKindStatefulSet {
		return fmt.Errorf("工作负载类型必须为 deployment 或 statefulset")
	}
	replicas, err := a.scaleManagedWorkload(ctx, application.Namespace, application.ApplicationName, currentKind, 0)
	if err != nil {
		return err
	}
	targetApplied := false
	restore := func(cause error) error {
		if targetApplied {
			if err := a.stopManagedWorkloadIfExists(ctx, application.Namespace, application.ApplicationName, targetKind); err != nil {
				return fmt.Errorf("%w；停止新工作负载失败: %v", cause, err)
			}
		}
		if restoreErr := a.restoreManagedWorkload(ctx, application.Namespace, application.ApplicationName, currentKind, replicas); restoreErr != nil {
			return fmt.Errorf("%w；恢复原工作负载失败: %v", cause, restoreErr)
		}
		return cause
	}
	if err := a.waitForWorkloadPodsStopped(ctx, application.Namespace, application.ApplicationName); err != nil {
		return restore(err)
	}
	target := application
	target.WorkloadKind = targetKind
	resources, err := RenderResources(target, spec)
	if err != nil {
		return restore(err)
	}
	if err := a.Apply(ctx, resources); err != nil {
		return restore(fmt.Errorf("创建新工作负载: %w", err))
	}
	targetApplied = true
	if err := a.WaitReady(ctx, target, spec); err != nil {
		return restore(fmt.Errorf("新工作负载未就绪: %w", err))
	}
	return nil
}

func (a *KubernetesApplier) scaleManagedWorkload(ctx context.Context, namespace, name, kind string, replicas int32) (int32, error) {
	if kind == WorkloadKindStatefulSet {
		statefulSet, err := a.workloads.GetStatefulSet(ctx, namespace, name)
		if err != nil {
			return 0, fmt.Errorf("读取 StatefulSet: %w", err)
		}
		if err := ensureManaged(statefulSet.Labels); err != nil {
			return 0, err
		}
		previous := int32(1)
		if statefulSet.Spec.Replicas != nil {
			previous = *statefulSet.Spec.Replicas
		}
		statefulSet.Spec.Replicas = &replicas
		if _, err := a.workloads.UpdateStatefulSet(ctx, statefulSet); err != nil {
			return 0, fmt.Errorf("缩容 StatefulSet: %w", err)
		}
		return previous, nil
	}
	deployment, err := a.workloads.GetDeployment(ctx, namespace, name)
	if err != nil {
		return 0, fmt.Errorf("读取 Deployment: %w", err)
	}
	if err := ensureManaged(deployment.Labels); err != nil {
		return 0, err
	}
	previous := int32(1)
	if deployment.Spec.Replicas != nil {
		previous = *deployment.Spec.Replicas
	}
	deployment.Spec.Replicas = &replicas
	if _, err := a.workloads.UpdateDeployment(ctx, deployment); err != nil {
		return 0, fmt.Errorf("缩容 Deployment: %w", err)
	}
	return previous, nil
}

func (a *KubernetesApplier) restoreManagedWorkload(ctx context.Context, namespace, name, kind string, replicas int32) error {
	_, err := a.scaleManagedWorkload(ctx, namespace, name, kind, replicas)
	return err
}

func (a *KubernetesApplier) stopManagedWorkloadIfExists(ctx context.Context, namespace, name, kind string) error {
	if kind == WorkloadKindStatefulSet {
		statefulSet, err := a.workloads.GetStatefulSet(ctx, namespace, name)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ensureManaged(statefulSet.Labels); err != nil {
			return err
		}
		zero := int32(0)
		statefulSet.Spec.Replicas = &zero
		_, err = a.workloads.UpdateStatefulSet(ctx, statefulSet)
		return err
	}
	deployment, err := a.workloads.GetDeployment(ctx, namespace, name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(deployment.Labels); err != nil {
		return err
	}
	zero := int32(0)
	deployment.Spec.Replicas = &zero
	_, err = a.workloads.UpdateDeployment(ctx, deployment)
	return err
}

func (a *KubernetesApplier) waitForWorkloadPodsStopped(ctx context.Context, namespace, name string) error {
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	selector := ApplicationNameLabel + "=" + name
	for {
		pods, err := a.diagnostics.ListPods(deadline, namespace, selector)
		if err != nil {
			return fmt.Errorf("检查旧 Pod: %w", err)
		}
		if len(pods) == 0 {
			return nil
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("等待旧 Pod 停止超时")
		case <-ticker.C:
		}
	}
}

// InspectReleasePods returns live runtime state for Pods created by a Release.
// Release labels are placed on the Pod template so rollouts stay traceable.
func (a *KubernetesApplier) InspectReleasePods(ctx context.Context, application ApplicationContext) (*model.ReleaseRuntime, error) {
	if a.diagnostics == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	selector := ApplicationNameLabel + "=" + application.ApplicationName + "," + ReleaseLabel + "=" + strconv.FormatUint(uint64(application.ReleaseSequence), 10)
	pods, err := a.diagnostics.ListPods(ctx, application.Namespace, selector)
	if err != nil {
		return nil, fmt.Errorf("读取关联 Pod: %w", err)
	}
	runtime := &model.ReleaseRuntime{Tracking: "exact", Pods: make([]model.ReleasePodRuntime, 0, len(pods))}
	pendingPodNames := make(map[string]struct{}, len(pods))
	diagnostics := make([]string, 0)
	for _, pod := range pods {
		status, diagnostic := releasePodRuntime(pod)
		runtime.Pods = append(runtime.Pods, status)
		if !status.Ready {
			pendingPodNames[pod.Name] = struct{}{}
		}
		if diagnostic != "" {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	sort.Slice(runtime.Pods, func(i, j int) bool { return runtime.Pods[i].Name < runtime.Pods[j].Name })
	if len(pendingPodNames) > 0 {
		if events, err := a.diagnostics.ListEvents(ctx, application.Namespace); err == nil {
			for _, event := range events {
				if event.Type != corev1.EventTypeWarning || event.InvolvedObject.Kind != "Pod" || event.Message == "" {
					continue
				}
				if _, exists := pendingPodNames[event.InvolvedObject.Name]; exists {
					diagnostics = append(diagnostics, fmt.Sprintf("Pod %s: %s: %s", event.InvolvedObject.Name, event.Reason, event.Message))
				}
			}
		}
	}
	runtime.Diagnostic = releaseRuntimeDiagnostic(diagnostics)
	return runtime, nil
}

func releasePodRuntime(pod corev1.Pod) (model.ReleasePodRuntime, string) {
	result := model.ReleasePodRuntime{Name: pod.Name, NodeName: pod.Spec.NodeName, Phase: string(pod.Status.Phase), CreatedAt: pod.CreationTimestamp.Time, Containers: make([]model.ReleaseContainerRuntime, 0, len(pod.Status.ContainerStatuses))}
	diagnostics := make([]string, 0)
	if pod.Status.Phase == corev1.PodFailed {
		diagnostics = append(diagnostics, fmt.Sprintf("Pod %s 已失败: %s", pod.Name, strings.TrimSpace(pod.Status.Message)))
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Message != "" {
			diagnostics = append(diagnostics, fmt.Sprintf("Pod %s 调度失败: %s", pod.Name, condition.Message))
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		container, diagnostic := releaseContainerRuntime(pod.Name, status)
		result.Containers = append(result.Containers, container)
		result.Restarts += status.RestartCount
		if diagnostic != "" {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	result.Ready = len(result.Containers) > 0 && pod.Status.Phase == corev1.PodRunning
	for _, container := range result.Containers {
		if !container.Ready {
			result.Ready = false
			break
		}
	}
	result.Diagnostic = releaseRuntimeDiagnostic(diagnostics)
	return result, result.Diagnostic
}

func releaseContainerRuntime(podName string, status corev1.ContainerStatus) (model.ReleaseContainerRuntime, string) {
	result := model.ReleaseContainerRuntime{Name: status.Name, Image: status.Image, Ready: status.Ready, RestartCount: status.RestartCount}
	diagnostic := ""
	if waiting := status.State.Waiting; waiting != nil {
		result.State, result.Reason, result.Message = "waiting", waiting.Reason, waiting.Message
		diagnostic = fmt.Sprintf("Pod %s 的容器 %s: %s", podName, status.Name, waiting.Reason)
		if waiting.Message != "" {
			diagnostic += ": " + waiting.Message
		}
	} else if terminated := status.State.Terminated; terminated != nil {
		result.State, result.Reason, result.Message = "terminated", terminated.Reason, terminated.Message
		result.ExitCode = int32Pointer(terminated.ExitCode)
		diagnostic = fmt.Sprintf("Pod %s 的容器 %s 已退出: %s (退出码 %d)", podName, status.Name, terminated.Reason, terminated.ExitCode)
	} else if status.State.Running != nil {
		result.State = "running"
	}
	if terminated := status.LastTerminationState.Terminated; terminated != nil {
		result.LastState, result.LastReason, result.LastExitCode = "terminated", terminated.Reason, int32Pointer(terminated.ExitCode)
		if status.RestartCount > 0 && terminated.ExitCode == 0 {
			lastExit := fmt.Sprintf("Pod %s 的容器 %s 曾正常退出 (退出码 0)，但已重启 %d 次", podName, status.Name, status.RestartCount)
			if diagnostic == "" {
				diagnostic = lastExit
			} else {
				diagnostic += "；" + lastExit
			}
		} else if diagnostic == "" && status.RestartCount > 0 {
			diagnostic = fmt.Sprintf("Pod %s 的容器 %s 上次退出: %s (退出码 %d)，已重启 %d 次", podName, status.Name, terminated.Reason, terminated.ExitCode, status.RestartCount)
		}
	}
	if diagnostic == "" && !status.Ready {
		diagnostic = fmt.Sprintf("Pod %s 的容器 %s 尚未就绪", podName, status.Name)
	}
	return result, diagnostic
}

func int32Pointer(value int32) *int32 {
	return &value
}

func releaseRuntimeDiagnostic(diagnostics []string) string {
	seen := make(map[string]struct{}, len(diagnostics))
	values := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		diagnostic = strings.TrimSpace(diagnostic)
		if diagnostic == "" {
			continue
		}
		if _, exists := seen[diagnostic]; exists {
			continue
		}
		seen[diagnostic] = struct{}{}
		values = append(values, diagnostic)
	}
	return redactReleaseDiagnostic(strings.Join(values, "；"), ReleaseSpec{})
}

func (a *KubernetesApplier) deploymentFailureDiagnostic(ctx context.Context, application ApplicationContext, spec ReleaseSpec) (string, bool) {
	selector := ApplicationNameLabel + "=" + application.ApplicationName
	if application.ReleaseSequence > 0 {
		selector += "," + ReleaseLabel + "=" + strconv.FormatUint(uint64(application.ReleaseSequence), 10)
	}
	pods, err := a.diagnostics.ListPods(ctx, application.Namespace, selector)
	if err != nil {
		return "", false
	}
	podNames := make(map[string]struct{}, len(pods))
	lastDiagnostic := ""
	for _, pod := range pods {
		podNames[pod.Name] = struct{}{}
		if pod.Status.Phase == corev1.PodFailed {
			return redactReleaseDiagnostic(fmt.Sprintf("Pod %s 已失败: %s", pod.Name, pod.Status.Message), spec), true
		}
		for _, status := range pod.Status.ContainerStatuses {
			if waiting := status.State.Waiting; waiting != nil && waiting.Reason != "" {
				diagnostic := fmt.Sprintf("Pod %s 的容器 %s: %s", pod.Name, status.Name, waiting.Reason)
				if waiting.Message != "" {
					diagnostic += ": " + waiting.Message
				}
				diagnostic = redactReleaseDiagnostic(diagnostic, spec)
				if terminalContainerWaitingReason(waiting.Reason) {
					return diagnostic, true
				}
				lastDiagnostic = diagnostic
			}
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Message != "" {
				lastDiagnostic = redactReleaseDiagnostic(fmt.Sprintf("Pod %s 调度失败: %s", pod.Name, condition.Message), spec)
			}
		}
	}
	events, err := a.diagnostics.ListEvents(ctx, application.Namespace)
	if err != nil {
		return lastDiagnostic, false
	}
	for _, event := range events {
		if event.Type != corev1.EventTypeWarning || event.InvolvedObject.Kind != "Pod" {
			continue
		}
		if _, exists := podNames[event.InvolvedObject.Name]; !exists || event.Message == "" {
			continue
		}
		lastDiagnostic = redactReleaseDiagnostic(fmt.Sprintf("Pod %s: %s: %s", event.InvolvedObject.Name, event.Reason, event.Message), spec)
	}
	return lastDiagnostic, false
}

func terminalContainerWaitingReason(reason string) bool {
	switch reason {
	case "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "CreateContainerConfigError", "CreateContainerError", "RunContainerError":
		return true
	default:
		return false
	}
}

func redactReleaseDiagnostic(detail string, spec ReleaseSpec) string {
	detail = strings.TrimSpace(detail)
	for _, credential := range []string{spec.RegistryCredential, spec.ImageVerificationCredential} {
		if credential = strings.TrimSpace(credential); credential != "" {
			detail = strings.ReplaceAll(detail, credential, "[REDACTED]")
		}
	}
	if len(detail) > 1000 {
		return detail[:1000]
	}
	return detail
}

func (a *KubernetesApplier) certificateReady(ctx context.Context, application ApplicationContext, endpoint EndpointSpec) (bool, error) {
	if a.diagnostics == nil {
		return false, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	certificateName := application.ApplicationName + "-tls"
	if endpoint.ManagedCertificateName != "" {
		certificateName = endpoint.ManagedCertificateName
	}
	ready, err := a.diagnostics.CertificateReady(ctx, application.Namespace, certificateName)
	if err != nil {
		return false, fmt.Errorf("读取 Certificate 状态: %w", err)
	}
	return ready, nil
}

func ensureManaged(labels map[string]string) error {
	if labels[ManagedByLabel] != ManagedByValue {
		return fmt.Errorf("同名资源不受 Cylism Manager 管理")
	}
	return nil
}
