package k8s

import (
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
	opsAgentNamespace     = "default"
	opsAgentName          = "cylism-ops-agent"
	opsAgentContainerName = "agent"
	opsAgentDefaultImage  = "crpi-c5u9bb8i5qxw1m72.cn-guangzhou.personal.cr.aliyuncs.com/surrychen/cylism-ops-agent:latest"
)

type OpsAgentConfig struct {
	NodeName         string `json:"node_name"`
	Storage          string `json:"storage"`
	StorageClassName string `json:"storage_class_name,omitempty"`
	Image            string `json:"image,omitempty"`
	Model            string `json:"model"`
	BaseURL          string `json:"base_url,omitempty"`
}

type OpsAgentStatus struct {
	State           string `json:"state"`
	Message         string `json:"message"`
	NodeName        string `json:"node_name,omitempty"`
	Image           string `json:"image,omitempty"`
	Model           string `json:"model,omitempty"`
	PVCName         string `json:"pvc_name,omitempty"`
	Storage         string `json:"storage,omitempty"`
	DesiredReplicas int32  `json:"desired_replicas"`
	UpdatedReplicas int32  `json:"updated_replicas"`
	ReadyReplicas   int32  `json:"ready_replicas"`
}

func (c *Client) OpsAgentStatus() *OpsAgentStatus {
	status := &OpsAgentStatus{State: "unavailable", Message: "Kubernetes 客户端未初始化"}
	if c == nil || c.Clientset == nil {
		return status
	}
	deployment, err := c.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(c.Ctx(), opsAgentName, metav1.GetOptions{})
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
	claim, claimErr := c.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace).Get(c.Ctx(), opsAgentName+"-audit", metav1.GetOptions{})
	if claimErr == nil {
		status.PVCName = claim.Name
		if size := claim.Spec.Resources.Requests.Storage(); size != nil {
			status.Storage = size.String()
		}
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

func (c *Client) InstallOpsAgent(config OpsAgentConfig, apiKey string) (*OpsAgentStatus, error) {
	if c == nil || c.Clientset == nil {
		return c.OpsAgentStatus(), fmt.Errorf("Kubernetes 客户端未初始化")
	}
	config.NodeName, config.Storage, config.Model = strings.TrimSpace(config.NodeName), strings.TrimSpace(config.Storage), strings.TrimSpace(config.Model)
	if config.NodeName == "" || config.Storage == "" || config.Model == "" || strings.TrimSpace(apiKey) == "" {
		return c.OpsAgentStatus(), fmt.Errorf("节点、存储容量、模型和 API Key 不能为空")
	}
	if _, err := resource.ParseQuantity(config.Storage); err != nil {
		return c.OpsAgentStatus(), fmt.Errorf("存储容量无效")
	}
	node, err := c.Clientset.CoreV1().Nodes().Get(c.Ctx(), config.NodeName, metav1.GetOptions{})
	if err != nil {
		return c.OpsAgentStatus(), fmt.Errorf("读取节点失败: %w", err)
	}
	if !nodeReady(node) {
		return c.OpsAgentStatus(), fmt.Errorf("节点 %s 未就绪", config.NodeName)
	}
	if config.Image == "" {
		config.Image = opsAgentDefaultImage
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
	return c.OpsAgentStatus(), nil
}

func (c *Client) UninstallOpsAgent() error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	for _, remove := range []func() error{
		func() error {
			return c.Clientset.AppsV1().Deployments(opsAgentNamespace).Delete(c.Ctx(), opsAgentName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Services(opsAgentNamespace).Delete(c.Ctx(), opsAgentName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ConfigMaps(opsAgentNamespace).Delete(c.Ctx(), opsAgentName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Secrets(opsAgentNamespace).Delete(c.Ctx(), opsAgentName+"-model", metav1.DeleteOptions{})
		},
	} {
		if err := remove(); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (c *Client) upsertOpsAgentPVC(config OpsAgentConfig) error {
	name := opsAgentName + "-audit"
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
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Labels: labels}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: map[string]string{"cylism.dev/runtime-config-version": time.Now().UTC().Format(time.RFC3339Nano)}}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, NodeSelector: map[string]string{"kubernetes.io/hostname": config.NodeName}, SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &readOnly, RunAsUser: &runAs, RunAsGroup: &runAs}, Containers: []corev1.Container{{Name: opsAgentContainerName, Image: config.Image, ImagePullPolicy: corev1.PullAlways, Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}, EnvFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: opsAgentName}}}}, Env: []corev1.EnvVar{{Name: "OPENAI_API_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: opsAgentName + "-model"}, Key: "api-key"}}}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("256Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi")}}, SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, ReadOnlyRootFilesystem: &readOnly, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}, VolumeMounts: []corev1.VolumeMount{{Name: "audit", MountPath: "/var/lib/cylism-ops-agent"}, {Name: "temp", MountPath: "/tmp"}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromString("http")}}}, LivenessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromString("http")}}}}}, Volumes: []corev1.Volume{{Name: "audit", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: opsAgentName + "-audit"}}}, {Name: "temp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}}}}}
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
