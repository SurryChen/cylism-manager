package runtime

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
)

const DefaultNamespace = "cylism-assistant"

const (
	RuntimeLabel        = "cylism.io/runtime"
	RuntimeIDLabel      = "cylism.io/runtime-id"
	RuntimeConfigKey    = "runtime.json"
	RuntimeSecretKey    = "api-key"
	RuntimeAPISecretKey = "runtime-api-key"
	NanobotGatewayPort  = 18790
	NanobotAPIPort      = 8900
	NanobotConfigPath   = "/data/.nanobot/config.json"
	RuntimeDefaultPort  = NanobotAPIPort
	PermissionFixInit   = "fix-perms"
)

type KubernetesManager struct {
	Client   *k8s.Client
	Registry *Registry
}

func NewKubernetesManager(client *k8s.Client, registries ...*Registry) *KubernetesManager {
	registry := BuiltinRegistry()
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
	}
	return &KubernetesManager{Client: client, Registry: registry}
}

func RuntimeNameValid(name string) bool {
	return name != "" && len(validation.IsDNS1123Label(name)) == 0
}

func NamespaceValid(namespace string) bool {
	return namespace != "" && len(validation.IsDNS1123Subdomain(namespace)) == 0
}

func (m *KubernetesManager) EnsureNamespace(ctx context.Context, namespace string) error {
	if m == nil || m.Client == nil || m.Client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}
	_, err := m.Client.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = m.Client.Clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue}}}, metav1.CreateOptions{})
	}
	if err != nil {
		return fmt.Errorf("准备 Runtime 命名空间 %q: %w", namespace, err)
	}
	return nil
}

func (m *KubernetesManager) Apply(ctx context.Context, instance *model.RuntimeInstance, apiKey, runtimeAPIKey string) error {
	if m == nil || m.Client == nil || m.Client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if !RuntimeNameValid(instance.Name) {
		return fmt.Errorf("Runtime 名称无效")
	}
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("Runtime 缺少模型 API 密钥")
	}
	if strings.TrimSpace(runtimeAPIKey) == "" {
		return fmt.Errorf("Runtime 缺少 Agent API 密钥")
	}
	registry := m.Registry
	if registry == nil {
		registry = BuiltinRegistry()
	}
	adapter, ok := registry.Get(instance.RuntimeType)
	if !ok {
		return fmt.Errorf("不支持的 Runtime 类型: %s", instance.RuntimeType)
	}
	if err := adapter.Validate(instance); err != nil {
		return err
	}
	if instance.Namespace == "" {
		instance.Namespace = DefaultNamespace
	}
	if instance.Port == 0 {
		instance.Port = adapter.Definition().DefaultPort
	}
	if instance.HealthPath == "" {
		instance.HealthPath = adapter.Definition().DefaultHealthPath
	}
	if instance.PVCName == "" {
		instance.PVCName = instance.Name + "-data"
	}
	if instance.Storage == "" {
		instance.Storage = "10Gi"
	}
	if err := m.EnsureNamespace(ctx, instance.Namespace); err != nil {
		return err
	}
	labels := map[string]string{
		k8s.ManagedByLabel:      k8s.ManagedByValue,
		k8s.InfrastructureLabel: k8s.InfrastructureRuntime,
		RuntimeLabel:            instance.Name,
		RuntimeIDLabel:          fmt.Sprintf("%d", instance.ID),
	}
	if err := m.applyPVC(ctx, instance, labels); err != nil {
		return err
	}
	config, err := adapter.Config(instance)
	if err != nil {
		return err
	}
	if _, err := m.Client.Clientset.CoreV1().ConfigMaps(instance.Namespace).Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: instance.Name + "-config", Namespace: instance.Namespace, Labels: labels}, Data: map[string]string{RuntimeConfigKey: config}}, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("创建 Runtime ConfigMap: %w", err)
		}
		current, getErr := m.Client.Clientset.CoreV1().ConfigMaps(instance.Namespace).Get(ctx, instance.Name+"-config", metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("读取 Runtime ConfigMap: %w", getErr)
		}
		current.Data = map[string]string{RuntimeConfigKey: config}
		current.Labels = labels
		if _, err = m.Client.Clientset.CoreV1().ConfigMaps(instance.Namespace).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("更新 Runtime ConfigMap: %w", err)
		}
	}
	if err := m.applySecret(ctx, instance, labels, apiKey, runtimeAPIKey); err != nil {
		return err
	}
	workload, err := adapter.Workload(instance)
	if err != nil {
		return err
	}
	instance.Port = workload.ServicePort
	instance.HealthPath = workload.HealthPath
	if err := m.applyService(ctx, instance, labels, workload); err != nil {
		return err
	}
	if err := m.applyDeployment(ctx, instance, labels, workload); err != nil {
		return err
	}
	instance.EndpointURL = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", instance.Name, instance.Namespace, instance.Port)
	return nil
}

func (m *KubernetesManager) Delete(ctx context.Context, instance *model.RuntimeInstance, deletePVC bool) error {
	if m == nil || m.Client == nil || m.Client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	options := metav1.DeleteOptions{}
	if err := m.Client.Clientset.AppsV1().Deployments(instance.Namespace).Delete(ctx, instance.Name, options); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("删除 Runtime Deployment: %w", err)
	}
	for _, name := range []string{instance.Name, instance.Name + "-config", instance.SecretName} {
		if name == "" {
			continue
		}
		if err := m.Client.Clientset.CoreV1().Services(instance.Namespace).Delete(ctx, name, options); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("删除 Runtime Service: %w", err)
		}
		if err := m.Client.Clientset.CoreV1().ConfigMaps(instance.Namespace).Delete(ctx, name, options); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("删除 Runtime ConfigMap: %w", err)
		}
		if err := m.Client.Clientset.CoreV1().Secrets(instance.Namespace).Delete(ctx, name, options); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("删除 Runtime Secret: %w", err)
		}
	}
	if deletePVC && instance.PVCName != "" {
		claim, getErr := m.Client.Clientset.CoreV1().PersistentVolumeClaims(instance.Namespace).Get(ctx, instance.PVCName, metav1.GetOptions{})
		if getErr == nil && (claim.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue || claim.Labels[RuntimeIDLabel] != fmt.Sprintf("%d", instance.ID)) {
			return fmt.Errorf("PVC %s/%s 不属于该 Runtime，拒绝删除", instance.Namespace, instance.PVCName)
		}
		if getErr != nil && !apierrors.IsNotFound(getErr) {
			return fmt.Errorf("检查 Runtime PVC: %w", getErr)
		}
		if err := m.Client.Clientset.CoreV1().PersistentVolumeClaims(instance.Namespace).Delete(ctx, instance.PVCName, options); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("删除 Runtime PVC: %w", err)
		}
	}
	return nil
}

func (m *KubernetesManager) DeploymentReady(ctx context.Context, instance *model.RuntimeInstance) (bool, error) {
	if m == nil || m.Client == nil || m.Client.Clientset == nil {
		return false, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	deployment, err := m.Client.Clientset.AppsV1().Deployments(instance.Namespace).Get(ctx, instance.Name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("读取 Runtime Deployment: %w", err)
	}
	return deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.AvailableReplicas >= 1 && deployment.Status.UnavailableReplicas == 0, nil
}

func (m *KubernetesManager) Health(ctx context.Context, instance *model.RuntimeInstance) (string, string) {
	base := strings.TrimRight(instance.EndpointURL, "/")
	if base == "" {
		base = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", instance.Name, instance.Namespace, instance.Port)
	}
	path := instance.HealthPath
	if path == "" {
		path = "/health"
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, base+"/"+strings.TrimLeft(path, "/"), nil)
	if err != nil {
		return model.RuntimeStatusFailed, err.Error()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return model.RuntimeStatusFailed, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return model.RuntimeStatusDegraded, fmt.Sprintf("健康检查返回 HTTP %d", resp.StatusCode)
	}
	return model.RuntimeStatusReady, "Runtime 健康检查通过"
}

func (m *KubernetesManager) applyPVC(ctx context.Context, instance *model.RuntimeInstance, labels map[string]string) error {
	storage, err := resource.ParseQuantity(instance.Storage)
	if err != nil || storage.Sign() <= 0 {
		return fmt.Errorf("Runtime 存储容量无效")
	}
	claims := m.Client.Clientset.CoreV1().PersistentVolumeClaims(instance.Namespace)
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: instance.PVCName, Namespace: instance.Namespace, Labels: labels}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: storage}}}}
	if class := strings.TrimSpace(instance.StorageClassName); class != "" {
		claim.Spec.StorageClassName = &class
	}
	current, err := claims.Get(ctx, instance.PVCName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = claims.Create(ctx, claim, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("创建 Runtime PVC: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 Runtime PVC: %w", err)
	}
	if current.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue || current.Labels[RuntimeIDLabel] != fmt.Sprintf("%d", instance.ID) {
		return fmt.Errorf("PVC %s/%s 已存在且不属于该 Runtime", instance.Namespace, instance.PVCName)
	}
	return nil
}

func (m *KubernetesManager) applySecret(ctx context.Context, instance *model.RuntimeInstance, labels map[string]string, apiKey, runtimeAPIKey string) error {
	name := instance.SecretName
	if name == "" {
		name = instance.Name + "-model"
		instance.SecretName = name
	}
	secrets := m.Client.Clientset.CoreV1().Secrets(instance.Namespace)
	values := map[string]string{}
	if apiKey != "" {
		values[RuntimeSecretKey] = apiKey
	}
	if runtimeAPIKey != "" {
		values[RuntimeAPISecretKey] = runtimeAPIKey
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: instance.Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, StringData: values}
	current, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = secrets.Create(ctx, secret, metav1.CreateOptions{})
	} else if err == nil {
		current.Labels = labels
		current.StringData = secret.StringData
		current.Data = nil
		_, err = secrets.Update(ctx, current, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("写入 Runtime Secret: %w", err)
	}
	return nil
}

func (m *KubernetesManager) applyService(ctx context.Context, instance *model.RuntimeInstance, labels map[string]string, workload WorkloadSpec) error {
	services := m.Client.Clientset.CoreV1().Services(instance.Namespace)
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: instance.Name, Namespace: instance.Namespace, Labels: labels}, Spec: corev1.ServiceSpec{Selector: labels, Ports: []corev1.ServicePort{{Name: "api", Port: workload.ServicePort, TargetPort: intstr.FromInt32(workload.ServicePort)}}}}
	current, err := services.Get(ctx, instance.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = services.Create(ctx, service, metav1.CreateOptions{})
	} else if err == nil {
		service.ResourceVersion = current.ResourceVersion
		service.Spec.ClusterIP = current.Spec.ClusterIP
		_, err = services.Update(ctx, service, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("写入 Runtime Service: %w", err)
	}
	return nil
}

func (m *KubernetesManager) applyDeployment(ctx context.Context, instance *model.RuntimeInstance, labels map[string]string, workload WorkloadSpec) error {
	replicas := int32(1)
	commonEnv := []corev1.EnvVar{
		{Name: "HOME", Value: "/data"},
		{Name: "CYLISM_RUNTIME_NAME", Value: instance.Name},
		{Name: "CYLISM_RUNTIME_TYPE", Value: instance.RuntimeType},
		{Name: "CYLISM_MODEL_API_KEY", ValueFrom: secretKeyRef(instance.SecretName, RuntimeSecretKey)},
		{Name: "CYLISM_RUNTIME_API_KEY", ValueFrom: secretKeyRef(instance.SecretName, RuntimeAPISecretKey)},
	}
	containerSecurity := &corev1.SecurityContext{AllowPrivilegeEscalation: boolPtr(false), ReadOnlyRootFilesystem: boolPtr(true), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}
	// Kubernetes does not apply fsGroup ownership to hostPath/local-path
	// volumes, so a workspace created by an earlier root-run deployment stays
	// root-owned and blocks the non-root runtime. The first init container
	// restores ownership before the config renderer and main containers start;
	// it is the only root container in the Pod and only needs CAP_CHOWN plus
	// CAP_DAC_READ_SEARCH to recurse into the private 0700 runtime directory.
	fixPermsInit := corev1.Container{
		Name:    PermissionFixInit,
		Image:   instance.Image,
		Command: []string{"sh", "-c"},
		Args:    []string{"mkdir -p /data/.nanobot /data/workspace && chown -R 1000:1000 /data"},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:                int64Ptr(0),
			RunAsGroup:               int64Ptr(0),
			RunAsNonRoot:             boolPtr(false),
			AllowPrivilegeEscalation: boolPtr(false),
			ReadOnlyRootFilesystem:   boolPtr(true),
			Capabilities:             &corev1.Capabilities{Add: []corev1.Capability{"CHOWN", "DAC_READ_SEARCH"}, Drop: []corev1.Capability{"ALL"}},
		},
		VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}},
	}
	initContainers := make([]corev1.Container, 0, len(workload.InitContainers)+1)
	initContainers = append(initContainers, fixPermsInit)
	for index := range workload.InitContainers {
		init := workload.InitContainers[index].DeepCopy()
		init.Env = append(init.Env, commonEnv...)
		init.VolumeMounts = append(init.VolumeMounts, corev1.VolumeMount{Name: "data", MountPath: "/data"}, corev1.VolumeMount{Name: "config", MountPath: "/etc/cylism", ReadOnly: true}, corev1.VolumeMount{Name: "tmp", MountPath: "/tmp"})
		init.SecurityContext = containerSecurity.DeepCopy()
		initContainers = append(initContainers, *init)
	}
	containers := make([]corev1.Container, len(workload.Containers))
	for index := range workload.Containers {
		containers[index] = *workload.Containers[index].DeepCopy()
		containers[index].Env = append(containers[index].Env, commonEnv...)
		containers[index].VolumeMounts = append(containers[index].VolumeMounts, corev1.VolumeMount{Name: "data", MountPath: "/data"}, corev1.VolumeMount{Name: "tmp", MountPath: "/tmp"})
		containers[index].SecurityContext = containerSecurity.DeepCopy()
	}
	automountServiceAccountToken := false
	uid := int64(1000)
	podSpec := corev1.PodSpec{
		AutomountServiceAccountToken: &automountServiceAccountToken,
		SecurityContext:              &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true), RunAsUser: &uid, RunAsGroup: &uid, FSGroup: &uid},
		InitContainers:               initContainers,
		Containers:                   containers,
		Volumes: []corev1.Volume{
			{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.PVCName}}},
			{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instance.Name + "-config"}}}},
			{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
	}
	if instance.NodeName != "" {
		podSpec.NodeSelector = map[string]string{corev1.LabelHostname: instance.NodeName}
	}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: instance.Name, Namespace: instance.Namespace, Labels: labels}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: podSpec}}}
	deployments := m.Client.Clientset.AppsV1().Deployments(instance.Namespace)
	current, err := deployments.Get(ctx, instance.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = deployments.Create(ctx, deployment, metav1.CreateOptions{})
	} else if err == nil {
		if current.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue || current.Labels[RuntimeIDLabel] != fmt.Sprintf("%d", instance.ID) {
			return fmt.Errorf("Deployment %s/%s 已存在且不属于该 Runtime", instance.Namespace, instance.Name)
		}
		deployment.ResourceVersion = current.ResourceVersion
		_, err = deployments.Update(ctx, deployment, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("写入 Runtime Deployment: %w", err)
	}
	return nil
}

func secretKeyRef(name, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: name}, Key: key}}
}

func boolPtr(value bool) *bool { return &value }

func int64Ptr(value int64) *int64 { return &value }
