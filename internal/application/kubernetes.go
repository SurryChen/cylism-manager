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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var certificateGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}

type KubernetesApplier struct {
	Client           *k8sclient.Client
	ReadinessTimeout time.Duration
}

func NewKubernetesApplier(client *k8sclient.Client) *KubernetesApplier {
	return &KubernetesApplier{Client: client, ReadinessTimeout: 2 * time.Minute}
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
	if a.Client == nil || a.Client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	namespace, err := a.Client.Clientset.CoreV1().Namespaces().Get(ctx, application.Namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("环境命名空间 %q 不存在，请在环境页面创建或同步命名空间", application.Namespace)
	}
	if err != nil {
		return fmt.Errorf("检查环境命名空间: %w", err)
	}
	if namespace.Status.Phase != corev1.NamespaceActive {
		return fmt.Errorf("环境命名空间 %q 未就绪", application.Namespace)
	}
	if err := a.ValidatePersistentVolumeClaims(application, spec); err != nil {
		return err
	}
	if err := a.ValidateFileMountSources(ctx, application, spec); err != nil {
		return err
	}
	if spec.Endpoint.Exposure != ExposurePublic {
		return nil
	}
	status, err := a.Client.DetectIngressController()
	if err != nil || status == nil || !status.Running {
		return fmt.Errorf("Ingress Controller 未就绪")
	}
	if spec.Endpoint.TLSEnabled {
		ok, err := a.Client.CheckCRD("certificates.cert-manager.io")
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
	for _, fileMount := range spec.FileMounts {
		switch fileMount.SourceType {
		case FileMountSourceApplicationConfig:
			if _, ok := spec.Config[fileMount.Key]; !ok {
				return fmt.Errorf("当前应用 ConfigMap 不包含键 %q", fileMount.Key)
			}
		case FileMountSourceSecret:
			secret, err := a.Client.Clientset.CoreV1().Secrets(application.Namespace).Get(ctx, fileMount.SourceName, metav1.GetOptions{})
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
			configMap, err := a.Client.Clientset.CoreV1().ConfigMaps(application.Namespace).Get(ctx, fileMount.SourceName, metav1.GetOptions{})
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
func (a *KubernetesApplier) ValidatePersistentVolumeClaims(application ApplicationContext, spec ReleaseSpec) error {
	if len(spec.Volumes) == 0 {
		return nil
	}
	if spec.Replicas != 1 {
		return fmt.Errorf("ReadWriteOnce PVC 仅支持单副本应用")
	}
	if spec.NodeName != "" {
		if _, err := a.Client.Clientset.CoreV1().Nodes().Get(a.Client.Ctx(), spec.NodeName, metav1.GetOptions{}); err != nil {
			return fmt.Errorf("检查部署节点 %q: %w", spec.NodeName, err)
		}
	}
	for _, volume := range spec.Volumes {
		claim, err := a.Client.GetManagedPVC(application.Namespace, volume.ClaimName, application.EnvironmentID)
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
	if resources.ImagePullSecret != nil {
		if err := a.applySecret(ctx, resources.ImagePullSecret); err != nil {
			return err
		}
	}
	if resources.ConfigMap != nil {
		if err := a.applyConfigMap(ctx, resources.ConfigMap); err != nil {
			return err
		}
	}
	if resources.Secret != nil {
		if err := a.applySecret(ctx, resources.Secret); err != nil {
			return err
		}
	}
	if resources.Deployment != nil {
		if err := a.applyDeployment(ctx, resources.Deployment); err != nil {
			return err
		}
	}
	if resources.StatefulSet != nil {
		if err := a.applyStatefulSet(ctx, resources.StatefulSet); err != nil {
			return err
		}
	}
	if err := a.applyService(ctx, resources.Service); err != nil {
		return err
	}
	if resources.Certificate != nil {
		if err := a.applyCertificate(ctx, resources.Certificate); err != nil {
			return err
		}
	}
	if resources.Ingress != nil {
		if err := a.applyIngress(ctx, resources.Ingress); err != nil {
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
	if a.Client == nil || a.Client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	publicEndpoints := make([]model.ApplicationEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Exposure == "" || endpoint.Exposure == ExposurePublic {
			publicEndpoints = append(publicEndpoints, endpoint)
		}
	}
	if len(publicEndpoints) == 0 {
		return a.RemoveEndpoint(ctx, application)
	}
	return a.applyIngress(ctx, applicationEndpointsIngress(application, publicEndpoints, servicePort))
}

func (a *KubernetesApplier) RemoveEndpoint(ctx context.Context, application ApplicationContext) error {
	if a.Client == nil || a.Client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	ingress, err := a.Client.Clientset.NetworkingV1().Ingresses(application.Namespace).Get(ctx, application.ApplicationName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(ingress.Labels); err != nil {
		return err
	}
	return a.Client.Clientset.NetworkingV1().Ingresses(application.Namespace).Delete(ctx, application.ApplicationName, metav1.DeleteOptions{})
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
		statefulSet, err := a.Client.Clientset.AppsV1().StatefulSets(application.Namespace).Get(ctx, application.ApplicationName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("读取 StatefulSet 就绪状态: %w", err)
		}
		return statefulSet.Status.ObservedGeneration >= statefulSet.Generation && statefulSet.Status.ReadyReplicas >= replicas && statefulSet.Status.CurrentReplicas >= replicas, nil
	}
	deployment, err := a.Client.Clientset.AppsV1().Deployments(application.Namespace).Get(ctx, application.ApplicationName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("读取 Deployment 就绪状态: %w", err)
	}
	return deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.UpdatedReplicas >= replicas && deployment.Status.AvailableReplicas >= replicas && deployment.Status.UnavailableReplicas == 0, nil
}

// MigrateWorkloadKind replaces one managed controller with the other while
// retaining the application name, Service and directly referenced PVCs.
func (a *KubernetesApplier) MigrateWorkloadKind(ctx context.Context, application ApplicationContext, spec ReleaseSpec, targetKind string) error {
	if a.Client == nil || a.Client.Clientset == nil {
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
		statefulSet, err := a.Client.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
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
		if _, err := a.Client.Clientset.AppsV1().StatefulSets(namespace).Update(ctx, statefulSet, metav1.UpdateOptions{}); err != nil {
			return 0, fmt.Errorf("缩容 StatefulSet: %w", err)
		}
		return previous, nil
	}
	deployment, err := a.Client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
	if _, err := a.Client.Clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
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
		statefulSet, err := a.Client.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
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
		_, err = a.Client.Clientset.AppsV1().StatefulSets(namespace).Update(ctx, statefulSet, metav1.UpdateOptions{})
		return err
	}
	deployment, err := a.Client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
	_, err = a.Client.Clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) waitForWorkloadPodsStopped(ctx context.Context, namespace, name string) error {
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	selector := ApplicationNameLabel + "=" + name
	for {
		pods, err := a.Client.Clientset.CoreV1().Pods(namespace).List(deadline, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return fmt.Errorf("检查旧 Pod: %w", err)
		}
		if len(pods.Items) == 0 {
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
	if a.Client == nil || a.Client.Clientset == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	selector := ApplicationNameLabel + "=" + application.ApplicationName + "," + ReleaseLabel + "=" + strconv.FormatUint(uint64(application.ReleaseSequence), 10)
	pods, err := a.Client.Clientset.CoreV1().Pods(application.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("读取关联 Pod: %w", err)
	}
	runtime := &model.ReleaseRuntime{Tracking: "exact", Pods: make([]model.ReleasePodRuntime, 0, len(pods.Items))}
	pendingPodNames := make(map[string]struct{}, len(pods.Items))
	diagnostics := make([]string, 0)
	for _, pod := range pods.Items {
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
		if events, err := a.Client.Clientset.CoreV1().Events(application.Namespace).List(ctx, metav1.ListOptions{}); err == nil {
			for _, event := range events.Items {
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
	pods, err := a.Client.Clientset.CoreV1().Pods(application.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", false
	}
	podNames := make(map[string]struct{}, len(pods.Items))
	lastDiagnostic := ""
	for _, pod := range pods.Items {
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
	events, err := a.Client.Clientset.CoreV1().Events(application.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return lastDiagnostic, false
	}
	for _, event := range events.Items {
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

func (a *KubernetesApplier) applyConfigMap(ctx context.Context, desired *corev1.ConfigMap) error {
	existing, err := a.Client.Clientset.CoreV1().ConfigMaps(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.CoreV1().ConfigMaps(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.Client.Clientset.CoreV1().ConfigMaps(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applySecret(ctx context.Context, desired *corev1.Secret) error {
	existing, err := a.Client.Clientset.CoreV1().Secrets(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.CoreV1().Secrets(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.Client.Clientset.CoreV1().Secrets(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applyDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	existing, err := a.Client.Clientset.AppsV1().Deployments(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.AppsV1().Deployments(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.Client.Clientset.AppsV1().Deployments(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applyStatefulSet(ctx context.Context, desired *appsv1.StatefulSet) error {
	existing, err := a.Client.Clientset.AppsV1().StatefulSets(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.AppsV1().StatefulSets(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.Client.Clientset.AppsV1().StatefulSets(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applyService(ctx context.Context, desired *corev1.Service) error {
	existing, err := a.Client.Clientset.CoreV1().Services(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.CoreV1().Services(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	desired.Spec.ClusterIP = existing.Spec.ClusterIP
	_, err = a.Client.Clientset.CoreV1().Services(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applyIngress(ctx context.Context, desired *networkingv1.Ingress) error {
	existing, err := a.Client.Clientset.NetworkingV1().Ingresses(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.NetworkingV1().Ingresses(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.Client.Clientset.NetworkingV1().Ingresses(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applyCertificate(ctx context.Context, desired *unstructured.Unstructured) error {
	dynamicClient, err := dynamic.NewForConfig(a.Client.Config)
	if err != nil {
		return err
	}
	resource := dynamicClient.Resource(certificateGVR).Namespace(desired.GetNamespace())
	existing, err := resource.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.GetLabels()); err != nil {
		return err
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	_, err = resource.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) certificateReady(ctx context.Context, application ApplicationContext, endpoint EndpointSpec) (bool, error) {
	dynamicClient, err := dynamic.NewForConfig(a.Client.Config)
	if err != nil {
		return false, err
	}
	certificateName := application.ApplicationName + "-tls"
	if endpoint.ManagedCertificateName != "" {
		certificateName = endpoint.ManagedCertificateName
	}
	certificate, err := dynamicClient.Resource(certificateGVR).Namespace(application.Namespace).Get(ctx, certificateName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("读取 Certificate 状态: %w", err)
	}
	conditions, _, _ := unstructured.NestedSlice(certificate.Object, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]interface{})
		if ok && condition["type"] == "Ready" && condition["status"] == "True" {
			return true, nil
		}
	}
	return false, nil
}

func ensureManaged(labels map[string]string) error {
	if labels[ManagedByLabel] != ManagedByValue {
		return fmt.Errorf("同名资源不受 Cylism Manager 管理")
	}
	return nil
}
