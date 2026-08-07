package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	OpsAgentRuntimeNamespace       = "cylism-assistant"
	LegacyOpsAgentRuntimeNamespace = "default"
	opsAgentNamespace              = OpsAgentRuntimeNamespace
	legacyOpsAgentNamespace        = LegacyOpsAgentRuntimeNamespace
	opsAgentName                   = "cylism-ops-agent"
	opsAgentContainerName          = "agent"
	opsAgentDefaultImage           = "crpi-c5u9bb8i5qxw1m72.cn-guangzhou.personal.cr.aliyuncs.com/surrychen/cylism-ops-agent:latest"
	opsAgentMigrationImage         = "registry.k8s.io/pause:3.10"
	opsAgentActivePVCLabel         = "cylism.io/assistant-active"
)

type OpsAgentConfig struct {
	NodeName         string `json:"node_name"`
	Storage          string `json:"storage"`
	StorageClassName string `json:"storage_class_name,omitempty"`
	Image            string `json:"image,omitempty"`
	Model            string `json:"model"`
	BaseURL          string `json:"base_url,omitempty"`
	AuditPVCName     string `json:"-"`
}

type OpsAgentStatus struct {
	State            string `json:"state"`
	Message          string `json:"message"`
	Namespace        string `json:"namespace,omitempty"`
	NodeName         string `json:"node_name,omitempty"`
	Image            string `json:"image,omitempty"`
	Model            string `json:"model,omitempty"`
	PVCName          string `json:"pvc_name,omitempty"`
	Storage          string `json:"storage,omitempty"`
	StorageClassName string `json:"storage_class_name,omitempty"`
	DesiredReplicas  int32  `json:"desired_replicas"`
	UpdatedReplicas  int32  `json:"updated_replicas"`
	ReadyReplicas    int32  `json:"ready_replicas"`
}

func (c *Client) OpsAgentStatus() *OpsAgentStatus {
	return c.opsAgentStatus(opsAgentNamespace)
}

// LegacyOpsAgentStatus reads the pre-migration Runtime only. It exists solely
// to make a controlled one-time migration possible.
func (c *Client) LegacyOpsAgentStatus() *OpsAgentStatus {
	return c.opsAgentStatus(legacyOpsAgentNamespace)
}

func (c *Client) opsAgentStatus(namespace string) *OpsAgentStatus {
	status := &OpsAgentStatus{State: "unavailable", Message: "Kubernetes 客户端未初始化"}
	if c == nil || c.Clientset == nil {
		return status
	}
	deployment, err := c.Clientset.AppsV1().Deployments(namespace).Get(c.Ctx(), opsAgentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		status.State = "not_installed"
		status.Message = "尚未安装智能助手 Runtime"
		return status
	}
	if err != nil {
		status.State = "degraded"
		status.Message = fmt.Sprintf("读取智能助手状态失败: %v", err)
		return status
	}
	status.DesiredReplicas = 1
	if deployment.Spec.Replicas != nil {
		status.DesiredReplicas = *deployment.Spec.Replicas
	}
	status.UpdatedReplicas = deployment.Status.UpdatedReplicas
	status.ReadyReplicas = deployment.Status.AvailableReplicas
	status.NodeName = deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"]
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == opsAgentContainerName {
			status.Image = container.Image
			for _, env := range container.Env {
				if env.Name == "OPS_AGENT_MODEL" {
					status.Model = env.Value
				}
			}
		}
	}
	claimName := opsAgentAuditPVCName(deployment)
	claim, claimErr := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(c.Ctx(), claimName, metav1.GetOptions{})
	if claimErr == nil {
		status.PVCName = claim.Name
		if size := claim.Spec.Resources.Requests.Storage(); size != nil {
			status.Storage = size.String()
		}
		status.StorageClassName = valueOrEmpty(claim.Spec.StorageClassName)
	}
	if deployment.Status.ObservedGeneration >= deployment.Generation && status.UpdatedReplicas >= status.DesiredReplicas && status.ReadyReplicas >= status.DesiredReplicas {
		status.State = "ready"
		status.Message = "智能助手 Runtime 已就绪"
	} else {
		status.State = "installing"
		if deployment.Status.ObservedGeneration < deployment.Generation || status.UpdatedReplicas < status.DesiredReplicas {
			status.Message = "智能助手 Runtime 正在应用新配置"
		} else {
			status.Message = "智能助手 Runtime 正在启动，等待工作负载就绪"
		}
	}
	return status
}

// PrepareOpsAgentMigration creates the target namespace, audit claim, and a
// reusable binding Pod so WaitForFirstConsumer storage is provisioned on the
// selected Runtime node before host-level data transfer begins.
func (c *Client) PrepareOpsAgentMigration(config OpsAgentConfig, migrationID string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if err := c.ValidateOpsAgentNode(config.NodeName); err != nil {
		return err
	}
	if err := c.ensureOpsAgentNamespace(); err != nil {
		return err
	}
	if err := c.upsertOpsAgentPVC(config); err != nil {
		return err
	}
	_, err := c.CreatePVCBindingPod(opsAgentNamespace, "ops-agent-"+migrationID, config.auditPVCName(), config.NodeName, opsAgentMigrationImage)
	return err
}

func (c *Client) WaitForOpsAgentPVCBound(ctx context.Context, claimNames ...string) (*PersistentVolumeClaimInfo, error) {
	claimName := opsAgentName + "-audit"
	if len(claimNames) > 0 && strings.TrimSpace(claimNames[0]) != "" {
		claimName = strings.TrimSpace(claimNames[0])
	}
	return c.WaitForPVCBound(ctx, opsAgentNamespace, claimName)
}

func (c *Client) DeleteOpsAgentMigrationBindingPod(migrationID string) error {
	return c.DeletePVCBindingPod(opsAgentNamespace, "ops-agent-"+migrationID)
}

func (c *Client) OpsAgentPVCInfo(namespace string) (*PersistentVolumeClaimInfo, error) {
	claimName := opsAgentName + "-audit"
	if namespace == opsAgentNamespace {
		if deployment, err := c.Clientset.AppsV1().Deployments(namespace).Get(c.Ctx(), opsAgentName, metav1.GetOptions{}); err == nil {
			claimName = opsAgentAuditPVCName(deployment)
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
	}
	return c.OpsAgentPVCInfoByName(namespace, claimName)
}

func (c *Client) OpsAgentPVCInfoByName(namespace, claimName string) (*PersistentVolumeClaimInfo, error) {
	return c.GetPVCInfo(namespace, strings.TrimSpace(claimName))
}

func (c *Client) ScaleLegacyOpsAgent(replicas int32) error {
	err := c.ScaleDeployment(legacyOpsAgentNamespace, opsAgentName, replicas)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) ScaleOpsAgent(replicas int32) error {
	err := c.ScaleDeployment(opsAgentNamespace, opsAgentName, replicas)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) InstallOpsAgent(config OpsAgentConfig, apiKey string) (*OpsAgentStatus, error) {
	if c == nil || c.Clientset == nil {
		return c.OpsAgentStatus(), fmt.Errorf("Kubernetes 客户端未初始化")
	}
	config.NodeName, config.Storage, config.Model, config.AuditPVCName = strings.TrimSpace(config.NodeName), strings.TrimSpace(config.Storage), strings.TrimSpace(config.Model), strings.TrimSpace(config.AuditPVCName)
	if config.NodeName == "" || config.Storage == "" || config.Model == "" || strings.TrimSpace(apiKey) == "" {
		return c.OpsAgentStatus(), fmt.Errorf("节点、存储容量、模型和 API Key 不能为空")
	}
	if _, err := resource.ParseQuantity(config.Storage); err != nil {
		return c.OpsAgentStatus(), fmt.Errorf("存储容量无效")
	}
	if err := c.ValidateOpsAgentNode(config.NodeName); err != nil {
		return c.OpsAgentStatus(), err
	}
	if activeClaim, err := c.activeOpsAgentAuditPVCName(); err == nil {
		if config.AuditPVCName == "" {
			config.AuditPVCName = activeClaim
		}
		if config.AuditPVCName == activeClaim {
			if claim, claimErr := c.OpsAgentPVCInfoByName(opsAgentNamespace, activeClaim); claimErr == nil && claim.IsLocal && claim.BoundNode != "" && claim.BoundNode != config.NodeName {
				return c.OpsAgentStatus(), fmt.Errorf("当前审计 PVC 已绑定节点 %s；请使用“迁移 Runtime 存储”操作迁移到节点 %s", claim.BoundNode, config.NodeName)
			}
		}
	} else if !apierrors.IsNotFound(err) {
		return c.OpsAgentStatus(), fmt.Errorf("读取 Runtime 审计 PVC 失败: %w", err)
	}
	if config.Image == "" {
		config.Image = opsAgentDefaultImage
	}
	if err := c.ensureOpsAgentNamespace(); err != nil {
		return c.OpsAgentStatus(), err
	}
	if err := c.upsertOpsAgentPVC(config); err != nil {
		return c.OpsAgentStatus(), err
	}
	if err := c.upsertOpsAgentSecret(apiKey); err != nil {
		return c.OpsAgentStatus(), err
	}
	if err := c.upsertOpsAgentConfig(config); err != nil {
		return c.OpsAgentStatus(), err
	}
	if err := c.upsertOpsAgentService(); err != nil {
		return c.OpsAgentStatus(), err
	}
	if err := c.upsertOpsAgentDeployment(config); err != nil {
		return c.OpsAgentStatus(), err
	}
	if err := c.MarkOpsAgentAuditPVCActive(config.auditPVCName()); err != nil {
		return c.OpsAgentStatus(), err
	}
	return c.OpsAgentStatus(), nil
}

func (c *Client) ensureOpsAgentNamespace() error {
	_, err := c.Clientset.CoreV1().Namespaces().Get(c.Ctx(), opsAgentNamespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.CoreV1().Namespaces().Create(c.Ctx(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   opsAgentNamespace,
			Labels: map[string]string{ManagedByLabel: ManagedByValue, "cylism.io/component": "assistant"},
		}}, metav1.CreateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建智能助手命名空间失败: %w", err)
	}
	return nil
}

// CleanupLegacyOpsAgent removes only the previous default-namespace Runtime
// resources after an audit migration has completed and the new Runtime is ready.
func (c *Client) CleanupLegacyOpsAgent() error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	for _, remove := range []func() error{
		func() error {
			return c.Clientset.AppsV1().Deployments(legacyOpsAgentNamespace).Delete(c.Ctx(), opsAgentName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Services(legacyOpsAgentNamespace).Delete(c.Ctx(), opsAgentName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ConfigMaps(legacyOpsAgentNamespace).Delete(c.Ctx(), opsAgentName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Secrets(legacyOpsAgentNamespace).Delete(c.Ctx(), opsAgentName+"-model", metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().PersistentVolumeClaims(legacyOpsAgentNamespace).Delete(c.Ctx(), opsAgentName+"-audit", metav1.DeleteOptions{})
		},
	} {
		if err := remove(); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (c *Client) UninstallOpsAgent() error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	// Uninstall both the current dedicated-namespace Runtime and any legacy
	// default-namespace Runtime. PVCs and namespaces are intentionally kept.
	for _, namespace := range []string{opsAgentNamespace, legacyOpsAgentNamespace} {
		if err := c.uninstallOpsAgentResources(namespace); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) uninstallOpsAgentResources(namespace string) error {
	pvcNames := map[string]struct{}{opsAgentName + "-audit": {}}
	if deployment, err := c.Clientset.AppsV1().Deployments(namespace).Get(c.Ctx(), opsAgentName, metav1.GetOptions{}); err == nil {
		pvcNames[opsAgentAuditPVCName(deployment)] = struct{}{}
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("读取 %s 命名空间 Runtime 失败: %w", namespace, err)
	}
	claims, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(c.Ctx(), metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s,%s=%s,%s=assistant", ManagedByLabel, ManagedByValue, InfrastructureLabel, InfrastructureOpsAgent, "cylism.io/component")})
	if err != nil {
		return fmt.Errorf("读取 %s 命名空间 Runtime PVC 失败: %w", namespace, err)
	}
	for _, claim := range claims.Items {
		pvcNames[claim.Name] = struct{}{}
	}
	removals := []func() error{
		func() error {
			return c.Clientset.AppsV1().Deployments(namespace).Delete(c.Ctx(), opsAgentName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Services(namespace).Delete(c.Ctx(), opsAgentName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ConfigMaps(namespace).Delete(c.Ctx(), opsAgentName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Secrets(namespace).Delete(c.Ctx(), opsAgentName+"-model", metav1.DeleteOptions{})
		},
	}
	for _, remove := range removals {
		if err := remove(); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("卸载 %s 命名空间 Runtime 资源失败: %w", namespace, err)
		}
	}
	for name := range pvcNames {
		if err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(c.Ctx(), name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("删除 %s 命名空间 Runtime PVC %s 失败: %w", namespace, name, err)
		}
	}
	return nil
}

func (c *Client) upsertOpsAgentPVC(config OpsAgentConfig) error {
	name := config.auditPVCName()
	claims := c.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace)
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: infrastructurePVCLabels(InfrastructureOpsAgent)}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(config.Storage)}}}}
	if config.StorageClassName != "" {
		claim.Spec.StorageClassName = &config.StorageClassName
	}
	existing, err := claims.Get(c.Ctx(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = claims.Create(c.Ctx(), claim, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if existing.Spec.Resources.Requests.Storage().Cmp(resource.MustParse(config.Storage)) > 0 {
		return fmt.Errorf("不能缩小现有助手审计 PVC")
	}
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	changed := false
	for key, value := range claim.Labels {
		if existing.Labels[key] != value {
			existing.Labels[key] = value
			changed = true
		}
	}
	if !changed {
		return nil
	}
	_, err = claims.Update(c.Ctx(), existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) upsertOpsAgentSecret(apiKey string) error {
	secrets := c.Clientset.CoreV1().Secrets(opsAgentNamespace)
	name := opsAgentName + "-model"
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: opsAgentLabels()}, Type: corev1.SecretTypeOpaque, StringData: map[string]string{"api-key": apiKey}}
	existing, err := secrets.Get(c.Ctx(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = secrets.Create(c.Ctx(), secret, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.StringData = secret.StringData
	existing.Labels = secret.Labels
	_, err = secrets.Update(c.Ctx(), existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) upsertOpsAgentConfig(config OpsAgentConfig) error {
	maps := c.Clientset.CoreV1().ConfigMaps(opsAgentNamespace)
	data := map[string]string{"CYLISM_API_URL": "http://cylism-manager.default.svc:8080", "OPS_AGENT_MODEL": config.Model, "CYLISM_API_TIMEOUT_SECONDS": "12"}
	if config.BaseURL != "" {
		data["OPENAI_BASE_URL"] = config.BaseURL
	}
	wanted := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Labels: opsAgentLabels()}, Data: data}
	existing, err := maps.Get(c.Ctx(), opsAgentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = maps.Create(c.Ctx(), wanted, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Data, existing.Labels = data, wanted.Labels
	_, err = maps.Update(c.Ctx(), existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) upsertOpsAgentService() error {
	services := c.Clientset.CoreV1().Services(opsAgentNamespace)
	wanted := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Labels: opsAgentLabels()}, Spec: corev1.ServiceSpec{Selector: opsAgentLabels(), Ports: []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromString("http")}}}}
	existing, err := services.Get(c.Ctx(), opsAgentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = services.Create(c.Ctx(), wanted, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec.Selector, existing.Spec.Ports = wanted.Spec.Selector, wanted.Spec.Ports
	_, err = services.Update(c.Ctx(), existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) upsertOpsAgentDeployment(config OpsAgentConfig) error {
	deployments := c.Clientset.AppsV1().Deployments(opsAgentNamespace)
	labels := opsAgentLabels()
	replicas := int32(1)
	runAs := int64(10001)
	no := false
	readOnly := true
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Labels: labels}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: map[string]string{"cylism.dev/runtime-config-version": time.Now().UTC().Format(time.RFC3339Nano)}}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, NodeSelector: map[string]string{"kubernetes.io/hostname": config.NodeName}, SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &readOnly, RunAsUser: &runAs, RunAsGroup: &runAs}, Containers: []corev1.Container{{Name: opsAgentContainerName, Image: config.Image, ImagePullPolicy: corev1.PullAlways, Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}, EnvFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: opsAgentName}}}}, Env: []corev1.EnvVar{{Name: "OPENAI_API_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: opsAgentName + "-model"}, Key: "api-key"}}}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("256Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi")}}, SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, ReadOnlyRootFilesystem: &readOnly, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}, VolumeMounts: []corev1.VolumeMount{{Name: "audit", MountPath: "/var/lib/cylism-ops-agent"}, {Name: "temp", MountPath: "/tmp"}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromString("http")}}}, LivenessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromString("http")}}}}}, Volumes: []corev1.Volume{{Name: "audit", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: config.auditPVCName()}}}, {Name: "temp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}}}}}
	existing, err := deployments.Get(c.Ctx(), opsAgentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = deployments.Create(c.Ctx(), deployment, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = deployment.Spec
	existing.Labels = deployment.Labels
	_, err = deployments.Update(c.Ctx(), existing, metav1.UpdateOptions{})
	return err
}

func opsAgentLabels() map[string]string {
	return map[string]string{"app.kubernetes.io/name": opsAgentName, "app.kubernetes.io/managed-by": "cylism-manager"}
}

func (c *Client) ValidateOpsAgentNode(name string) error {
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), strings.TrimSpace(name), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取目标节点失败: %w", err)
	}
	if !nodeReady(node) {
		return fmt.Errorf("目标节点 %s 未就绪", name)
	}
	return nil
}

func (c *Client) SwitchOpsAgentAuditPVC(sourceClaim, targetClaim, targetNode string, replicas int32) error {
	deployment, err := c.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(c.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if opsAgentAuditPVCName(deployment) != targetClaim {
		if _, err := c.ReplaceDeploymentPVCNode(opsAgentNamespace, opsAgentName, sourceClaim, targetClaim, targetNode); err != nil {
			return err
		}
	} else if deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != targetNode {
		if deployment.Spec.Template.Spec.NodeSelector == nil {
			deployment.Spec.Template.Spec.NodeSelector = map[string]string{}
		}
		deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] = targetNode
		if _, err := c.Clientset.AppsV1().Deployments(opsAgentNamespace).Update(c.Ctx(), deployment, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	if err := c.MarkOpsAgentAuditPVCActive(targetClaim); err != nil {
		return err
	}
	return c.ScaleOpsAgent(replicas)
}

func (c *Client) RestoreOpsAgentAuditPVC(sourceClaim, targetClaim, sourceNode string, replicas int32) error {
	deployment, err := c.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(c.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if opsAgentAuditPVCName(deployment) == targetClaim {
		return c.SwitchOpsAgentAuditPVC(targetClaim, sourceClaim, sourceNode, replicas)
	}
	return c.ScaleOpsAgent(replicas)
}

func (c *Client) DeleteOpsAgentAuditPVC(name string) error {
	err := c.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace).Delete(c.Ctx(), strings.TrimSpace(name), metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) MarkOpsAgentAuditPVCActive(name string) error {
	claim, err := c.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace).Get(c.Ctx(), strings.TrimSpace(name), metav1.GetOptions{})
	if err != nil {
		return err
	}
	if claim.Labels == nil {
		claim.Labels = map[string]string{}
	}
	if claim.Labels[opsAgentActivePVCLabel] == "true" {
		return nil
	}
	claim.Labels[opsAgentActivePVCLabel] = "true"
	_, err = c.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace).Update(c.Ctx(), claim, metav1.UpdateOptions{})
	return err
}

func (c *Client) activeOpsAgentAuditPVCName() (string, error) {
	deployment, err := c.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(c.Ctx(), opsAgentName, metav1.GetOptions{})
	if err == nil {
		return opsAgentAuditPVCName(deployment), nil
	}
	if !apierrors.IsNotFound(err) {
		return "", err
	}
	claims, listErr := c.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace).List(c.Ctx(), metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s,%s=assistant,%s=true", ManagedByLabel, ManagedByValue, "cylism.io/component", opsAgentActivePVCLabel)})
	if listErr != nil {
		return "", listErr
	}
	if len(claims.Items) == 0 {
		return "", err
	}
	if len(claims.Items) > 1 {
		return "", fmt.Errorf("检测到多个活动助手审计 PVC，无法确定要恢复的存储")
	}
	return claims.Items[0].Name, nil
}

func (c OpsAgentConfig) auditPVCName() string {
	if name := strings.TrimSpace(c.AuditPVCName); name != "" {
		return name
	}
	return opsAgentName + "-audit"
}

func opsAgentAuditPVCName(deployment *appsv1.Deployment) string {
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "audit" && volume.PersistentVolumeClaim != nil && strings.TrimSpace(volume.PersistentVolumeClaim.ClaimName) != "" {
			return volume.PersistentVolumeClaim.ClaimName
		}
	}
	return opsAgentName + "-audit"
}
