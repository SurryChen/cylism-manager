package api

import (
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

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const managedOCIRegistryResourceName = "cylism-oci-registry"

type ManagedOCIRegistryHandler struct {
	store     *store.Store
	encKey    []byte
	applyNode nodeRegistryMirrorApplier
}

type managedOCIRegistryRequest struct {
	Name                string `json:"name"`
	Namespace           string `json:"namespace"`
	Endpoint            string `json:"endpoint"`
	RegistryImage       string `json:"registry_image"`
	DataNode            string `json:"data_node"`
	DataPath            string `json:"data_path"`
	InsecureHTTP        bool   `json:"insecure_http"`
	ConfirmInsecureHTTP bool   `json:"confirm_insecure_http"`
	TLSSecretName       string `json:"tls_secret_name"`
	PullUsername        string `json:"pull_username"`
	PullPassword        string `json:"pull_password"`
	ProjectIDs          []uint `json:"project_ids"`
}
type managedOCIRegistryApplyRequest struct {
	ServerIDs []uint `json:"server_ids"`
}

func NewManagedOCIRegistryHandler(s *store.Store, encKey []byte) *ManagedOCIRegistryHandler {
	return &ManagedOCIRegistryHandler{store: s, encKey: encKey}
}

func (h *ManagedOCIRegistryHandler) List(c *gin.Context) {
	registries, err := h.store.ListManagedOCIRegistries()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取受管制品库失败")
		return
	}
	for i := range registries {
		h.refreshStatus(c.Request.Context(), &registries[i])
	}
	model.Success(c, registries)
}

func (h *ManagedOCIRegistryHandler) Get(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "制品库 ID 无效")
		return
	}
	registry, err := h.store.GetManagedOCIRegistry(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "受管制品库不存在")
		return
	}
	h.refreshStatus(c.Request.Context(), registry)
	model.Success(c, registry)
}

func (h *ManagedOCIRegistryHandler) Create(c *gin.Context) {
	if !managedRegistryK8sReady(c) {
		return
	}
	var request managedOCIRegistryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "制品库配置无效")
		return
	}
	registry, host, err := h.registryFromRequest(request, nil)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if existing, err := h.store.ListManagedOCIRegistries(); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取现有制品库失败")
		return
	} else if len(existing) > 0 {
		model.Error(c, http.StatusConflict, model.CodeConflict, "首期仅支持一个受管制品库")
		return
	}
	if err := h.ensureEndpointOwnership(registry.Endpoint); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, err.Error())
		return
	}
	if err := h.ensureResourcesAvailable(c.Request.Context(), registry.Namespace, registry.ResourceName); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, err.Error())
		return
	}
	if err := h.ensureTLSSecret(c.Request.Context(), registry); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	registry.EncryptedCredential, err = crypto.Encrypt(h.encKey, request.PullPassword)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "加密制品库凭据失败")
		return
	}
	registry.CreatedBy = getUserID(c)
	imageRegistry, mirror := managedRegistryAssociations(registry)
	if err := h.store.CreateManagedOCIRegistry(registry, imageRegistry, mirror, request.ProjectIDs); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "保存制品库配置失败，请检查名称、地址和项目授权")
		return
	}
	if err := h.applyResources(c.Request.Context(), registry, host, request.PullPassword); err != nil {
		registry.Status, registry.LastError = "failed", truncateManagedRegistryError(err)
		_ = h.store.UpdateManagedOCIRegistry(registry)
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "部署制品库失败: "+registry.LastError)
		return
	}
	registry.Status, registry.LastError, registry.CredentialConfigured = "deploying", "", true
	_ = h.store.UpdateManagedOCIRegistry(registry)
	model.Success(c, registry)
}

func (h *ManagedOCIRegistryHandler) Update(c *gin.Context) {
	if !managedRegistryK8sReady(c) {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "制品库 ID 无效")
		return
	}
	current, err := h.store.GetManagedOCIRegistry(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "受管制品库不存在")
		return
	}
	var request managedOCIRegistryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "制品库配置无效")
		return
	}
	registry, host, err := h.registryFromRequest(request, current)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if registry.Endpoint != current.Endpoint {
		model.Error(c, http.StatusConflict, model.CodeConflict, "首期不支持变更制品库地址，请新建并迁移镜像")
		return
	}
	if err := h.ensureTLSSecret(c.Request.Context(), registry); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	password := strings.TrimSpace(request.PullPassword)
	if password != "" {
		registry.EncryptedCredential, err = crypto.Encrypt(h.encKey, password)
	} else {
		password, err = crypto.Decrypt(h.encKey, current.EncryptedCredential)
	}
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取或加密制品库凭据失败")
		return
	}
	if err := h.applyResources(c.Request.Context(), registry, host, password); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "更新制品库失败: "+truncateManagedRegistryError(err))
		return
	}
	if err := h.updateAssociations(registry, request.ProjectIDs); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "更新关联制品库配置失败")
		return
	}
	registry.Status, registry.LastError, registry.CredentialConfigured = "deploying", "", true
	if err := h.store.UpdateManagedOCIRegistry(registry); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存制品库配置失败")
		return
	}
	model.Success(c, registry)
}

func (h *ManagedOCIRegistryHandler) updateAssociations(registry *model.ManagedOCIRegistry, projectIDs []uint) error {
	if registry.ImageRegistryID == nil || registry.NodeRegistryMirrorID == nil {
		return errors.New("受管制品库关联记录缺失")
	}
	imageRegistry, mirror := managedRegistryAssociations(registry)
	imageRegistry.ID, mirror.ID = *registry.ImageRegistryID, *registry.NodeRegistryMirrorID
	if err := h.store.UpdateImageRegistry(imageRegistry, projectIDs); err != nil {
		return err
	}
	return h.store.UpdateNodeRegistryMirror(mirror)
}

func (h *ManagedOCIRegistryHandler) ApplyNodeAccess(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "制品库 ID 无效")
		return
	}
	var request managedOCIRegistryApplyRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.ServerIDs) == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "至少选择一个集群节点")
		return
	}
	registry, err := h.store.GetManagedOCIRegistry(id)
	if err != nil || registry.NodeRegistryMirrorID == nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "受管制品库不存在")
		return
	}
	mirror, err := h.store.GetNodeRegistryMirror(*registry.NodeRegistryMirrorID)
	if err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "制品库节点镜像源配置缺失")
		return
	}
	nodeHandler := NewNodeRegistryMirrorHandler(h.store, h.encKey)
	content, err := nodeHandler.renderK3sRegistries()
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "生成节点镜像源配置失败: "+err.Error())
		return
	}
	servers, err := h.store.ListServers()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取服务器失败")
		return
	}
	byID := make(map[uint]model.Server, len(servers))
	for _, server := range servers {
		if server.ClusterRole != "" {
			byID[server.ID] = server
		}
	}
	failed, seen := 0, map[uint]struct{}{}
	for _, serverID := range request.ServerIDs {
		if _, duplicate := seen[serverID]; duplicate {
			continue
		}
		seen[serverID] = struct{}{}
		server, ok := byID[serverID]
		if !ok {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, fmt.Sprintf("节点 %d 不是可应用的集群节点", serverID))
			return
		}
		apply := h.applyNode
		if apply == nil {
			apply = nodeHandler.applyToNode
		}
		status, detail := apply(&server, content)
		if status != "success" {
			failed++
		}
		now := time.Now()
		_ = h.store.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: mirror.ID, ServerID: server.ID, Status: status, Detail: truncateManagedRegistryError(errors.New(detail)), AppliedAt: &now})
	}
	now := time.Now()
	mirror.LastAppliedAt = &now
	if failed == 0 {
		mirror.LastApplyStatus, mirror.LastApplyError = "succeeded", ""
	} else {
		mirror.LastApplyStatus, mirror.LastApplyError = "failed", fmt.Sprintf("%d 个节点未能应用制品库配置", failed)
	}
	_ = h.store.UpdateNodeRegistryMirror(mirror)
	updated, _ := h.store.GetManagedOCIRegistry(registry.ID)
	model.SuccessWithMessage(c, updated, "节点制品库配置已提交")
}

func (h *ManagedOCIRegistryHandler) Delete(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "制品库 ID 无效")
		return
	}
	var request struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || !request.Confirm {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "删除受管制品库必须明确确认；节点上的数据目录不会被删除")
		return
	}
	registry, err := h.store.GetManagedOCIRegistry(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "受管制品库不存在")
		return
	}
	releases, defaults, err := h.store.CountManagedOCIRegistryReferences(id)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "检查制品库引用失败")
		return
	}
	if releases > 0 || defaults > 0 {
		model.Error(c, http.StatusConflict, model.CodeConflict, fmt.Sprintf("制品库仍被 %d 个发布记录和 %d 个项目默认仓库引用", releases, defaults))
		return
	}
	if K8s != nil && K8s.Clientset != nil {
		ctx := c.Request.Context()
		_ = K8s.Clientset.NetworkingV1().Ingresses(registry.Namespace).Delete(ctx, registry.ResourceName, metav1.DeleteOptions{})
		_ = K8s.Clientset.CoreV1().Services(registry.Namespace).Delete(ctx, registry.ResourceName, metav1.DeleteOptions{})
		_ = K8s.Clientset.AppsV1().Deployments(registry.Namespace).Delete(ctx, registry.ResourceName, metav1.DeleteOptions{})
		_ = K8s.Clientset.CoreV1().Secrets(registry.Namespace).Delete(ctx, registry.ResourceName+"-auth", metav1.DeleteOptions{})
	}
	if registry.NodeRegistryMirrorID != nil {
		_ = h.store.DeleteNodeRegistryMirror(*registry.NodeRegistryMirrorID)
	}
	if registry.ImageRegistryID != nil {
		_ = h.store.DeleteImageRegistry(*registry.ImageRegistryID)
	}
	if err := h.store.DeleteManagedOCIRegistry(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "删除受管制品库记录失败")
		return
	}
	model.SuccessWithMessage(c, gin.H{"id": id, "data_path": registry.DataPath}, "制品库资源已删除，节点数据目录已保留")
}

func managedRegistryK8sReady(c *gin.Context) bool {
	if K8s != nil && K8s.Clientset != nil {
		return true
	}
	model.Error(c, http.StatusServiceUnavailable, model.CodeInternalError, "Kubernetes 集群未连接")
	return false
}

func (h *ManagedOCIRegistryHandler) registryFromRequest(request managedOCIRegistryRequest, current *model.ManagedOCIRegistry) (*model.ManagedOCIRegistry, string, error) {
	name, namespace := strings.TrimSpace(request.Name), strings.TrimSpace(request.Namespace)
	endpoint, host, err := normalizeManagedRegistryEndpoint(request.Endpoint)
	if err != nil {
		return nil, "", err
	}
	image, node, path := strings.TrimSpace(request.RegistryImage), strings.TrimSpace(request.DataNode), strings.TrimSpace(request.DataPath)
	username, password := strings.TrimSpace(request.PullUsername), strings.TrimSpace(request.PullPassword)
	if name == "" || len(name) > 128 || namespace == "" || len(namespace) > 253 {
		return nil, "", errors.New("制品库名称和命名空间必填，且长度不能超过限制")
	}
	if image == "" || strings.ContainsAny(image, " \t\r\n") {
		return nil, "", errors.New("Registry 镜像地址无效")
	}
	if node == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return nil, "", errors.New("数据节点必填，数据路径必须为不含 .. 的绝对路径")
	}
	if username == "" || strings.ContainsAny(username, ":\r\n") {
		return nil, "", errors.New("拉取账号必填，且不能包含冒号或换行")
	}
	if (current == nil && len(password) < 12) || (current != nil && password != "" && len(password) < 12) {
		return nil, "", errors.New("拉取密码至少需要 12 个字符")
	}
	if request.InsecureHTTP && !request.ConfirmInsecureHTTP {
		return nil, "", errors.New("使用 HTTP 制品库必须明确确认明文镜像和凭据传输风险")
	}
	if !request.InsecureHTTP && strings.TrimSpace(request.TLSSecretName) == "" {
		return nil, "", errors.New("HTTPS 制品库必须提供已存在的 TLS Secret")
	}
	registry := &model.ManagedOCIRegistry{Name: name, Namespace: namespace, ResourceName: managedOCIRegistryResourceName, Endpoint: endpoint, RegistryImage: image, DataNode: node, DataPath: path, InsecureHTTP: request.InsecureHTTP, TLSSecretName: strings.TrimSpace(request.TLSSecretName), PullUsername: username, Status: "pending"}
	if current != nil {
		registry.ID, registry.CreatedAt, registry.CreatedBy, registry.EncryptedCredential = current.ID, current.CreatedAt, current.CreatedBy, current.EncryptedCredential
		registry.ImageRegistryID, registry.NodeRegistryMirrorID = current.ImageRegistryID, current.NodeRegistryMirrorID
	}
	return registry, host, nil
}

func normalizeManagedRegistryEndpoint(value string) (endpoint, host string, err error) {
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

func managedRegistryAssociations(registry *model.ManagedOCIRegistry) (*model.ImageRegistry, *model.NodeRegistryMirror) {
	scheme := "https"
	if registry.InsecureHTTP {
		scheme = "http"
	}
	endpoints, _ := json.Marshal([]string{scheme + "://" + registry.Endpoint})
	name := "受管制品库 · " + registry.Name
	return &model.ImageRegistry{Name: name, Endpoint: registry.Endpoint, AuthType: registryAuthBasic, Username: registry.PullUsername, Credential: registry.EncryptedCredential, Enabled: true, CreatedBy: registry.CreatedBy, ManagedRegistryID: &registry.ID}, &model.NodeRegistryMirror{Name: name, Registry: registry.Endpoint, Endpoints: string(endpoints), Username: registry.PullUsername, Credential: registry.EncryptedCredential, Enabled: true, CreatedBy: registry.CreatedBy, ManagedRegistryID: &registry.ID}
}

func (h *ManagedOCIRegistryHandler) ensureEndpointOwnership(endpoint string) error {
	if registry, err := h.store.GetImageRegistryByEndpoint(endpoint); err == nil {
		if registry.ManagedRegistryID == nil {
			return fmt.Errorf("镜像仓库地址已由非受管记录 %q 使用", registry.Name)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("读取镜像仓库地址占用失败: %w", err)
	}
	if mirror, err := h.store.GetNodeRegistryMirrorByRegistry(endpoint); err == nil {
		if mirror.ManagedRegistryID == nil {
			return fmt.Errorf("节点镜像源地址已由非受管记录 %q 使用", mirror.Name)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("读取节点镜像源地址占用失败: %w", err)
	}
	return nil
}

func (h *ManagedOCIRegistryHandler) ensureResourcesAvailable(ctx context.Context, namespace, name string) error {
	checks := []struct {
		kind string
		get  func() (metav1.Object, error)
	}{
		{"Deployment", func() (metav1.Object, error) {
			return K8s.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		}}, {"Service", func() (metav1.Object, error) {
			return K8s.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		}}, {"Ingress", func() (metav1.Object, error) {
			return K8s.Clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		}}, {"认证 Secret", func() (metav1.Object, error) {
			return K8s.Clientset.CoreV1().Secrets(namespace).Get(ctx, name+"-auth", metav1.GetOptions{})
		}},
	}
	for _, check := range checks {
		resource, err := check.get()
		if err == nil && resource.GetLabels()["cylism.io/managed-registry"] != "true" {
			return fmt.Errorf("同名%s不属于 Cylism Manager，不能接管", check.kind)
		}
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (h *ManagedOCIRegistryHandler) ensureTLSSecret(ctx context.Context, registry *model.ManagedOCIRegistry) error {
	if registry.InsecureHTTP {
		return nil
	}
	secret, err := K8s.Clientset.CoreV1().Secrets(registry.Namespace).Get(ctx, registry.TLSSecretName, metav1.GetOptions{})
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

func (h *ManagedOCIRegistryHandler) applyResources(ctx context.Context, registry *model.ManagedOCIRegistry, host, password string) error {
	if err := h.ensureNamespace(ctx, registry.Namespace); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("生成制品库认证凭据失败")
	}
	labels := managedRegistryLabels(registry)
	if err := upsertManagedSecret(ctx, managedRegistryAuthSecret(registry, labels, hash)); err != nil {
		return err
	}
	if err := upsertManagedDeployment(ctx, managedRegistryDeployment(registry, labels)); err != nil {
		return err
	}
	if err := upsertManagedService(ctx, managedRegistryService(registry, labels)); err != nil {
		return err
	}
	return upsertManagedIngress(ctx, registry, host, labels)
}

func managedRegistryLabels(registry *model.ManagedOCIRegistry) map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "app.kubernetes.io/name": registry.ResourceName, "cylism.io/managed-registry": "true"}
}
func managedRegistryAuthSecret(registry *model.ManagedOCIRegistry, labels map[string]string, hash []byte) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: registry.ResourceName + "-auth", Namespace: registry.Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"htpasswd": []byte(registry.PullUsername + ":" + string(hash) + "\n")}}
}
func managedRegistryDeployment(registry *model.ManagedOCIRegistry, labels map[string]string) *appsv1.Deployment {
	replicas := int32(1)
	hostPathType := corev1.HostPathDirectoryOrCreate
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: registry.ResourceName, Namespace: registry.Namespace, Labels: labels}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: registry.DataNode}, Volumes: []corev1.Volume{{Name: "storage", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: registry.DataPath, Type: &hostPathType}}}, {Name: "auth", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: registry.ResourceName + "-auth"}}}}, Containers: []corev1.Container{{Name: "registry", Image: registry.RegistryImage, Ports: []corev1.ContainerPort{{Name: "registry", ContainerPort: 5000}}, Env: []corev1.EnvVar{{Name: "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", Value: "/var/lib/registry"}, {Name: "REGISTRY_AUTH", Value: "htpasswd"}, {Name: "REGISTRY_AUTH_HTPASSWD_REALM", Value: "Cylism Registry"}, {Name: "REGISTRY_AUTH_HTPASSWD_PATH", Value: "/auth/htpasswd"}}, VolumeMounts: []corev1.VolumeMount{{Name: "storage", MountPath: "/var/lib/registry"}, {Name: "auth", MountPath: "/auth", ReadOnly: true}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/v2/", Port: intstr.FromInt(5000)}}, InitialDelaySeconds: 3, PeriodSeconds: 5}}}}}}}
}
func managedRegistryService(registry *model.ManagedOCIRegistry, labels map[string]string) *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: registry.ResourceName, Namespace: registry.Namespace, Labels: labels}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: labels, Ports: []corev1.ServicePort{{Name: "registry", Port: 5000, TargetPort: intstr.FromInt(5000)}}}}
}

func (h *ManagedOCIRegistryHandler) ensureNamespace(ctx context.Context, namespace string) error {
	_, err := K8s.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	_, err = K8s.Clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "cylism-manager"}}}, metav1.CreateOptions{})
	return err
}
func upsertManagedSecret(ctx context.Context, desired *corev1.Secret) error {
	resources := K8s.Clientset.CoreV1().Secrets(desired.Namespace)
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
func upsertManagedDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	resources := K8s.Clientset.AppsV1().Deployments(desired.Namespace)
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
func upsertManagedService(ctx context.Context, desired *corev1.Service) error {
	resources := K8s.Clientset.CoreV1().Services(desired.Namespace)
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
func upsertManagedIngress(ctx context.Context, registry *model.ManagedOCIRegistry, host string, labels map[string]string) error {
	desired := managedRegistryIngress(registry, host, labels)
	resources := K8s.Clientset.NetworkingV1().Ingresses(desired.Namespace)
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
func managedRegistryIngress(registry *model.ManagedOCIRegistry, host string, labels map[string]string) *networkingv1.Ingress {
	pathType, ingressClass := networkingv1.PathTypePrefix, "traefik"
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: registry.ResourceName, Namespace: registry.Namespace, Labels: labels}, Spec: networkingv1.IngressSpec{IngressClassName: &ingressClass, Rules: []networkingv1.IngressRule{{Host: host, IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &pathType, Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: registry.ResourceName, Port: networkingv1.ServiceBackendPort{Number: 5000}}}}}}}}}}}
	if !registry.InsecureHTTP {
		ingress.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{host}, SecretName: registry.TLSSecretName}}
	}
	return ingress
}

func (h *ManagedOCIRegistryHandler) refreshStatus(ctx context.Context, registry *model.ManagedOCIRegistry) {
	if K8s == nil || K8s.Clientset == nil {
		return
	}
	status, detail := "degraded", "Registry Deployment 不存在"
	deployment, err := K8s.Clientset.AppsV1().Deployments(registry.Namespace).Get(ctx, registry.ResourceName, metav1.GetOptions{})
	if err == nil && deployment.Status.ReadyReplicas > 0 {
		status, detail = "ready", ""
		scheme := "https"
		if registry.InsecureHTTP {
			scheme = "http"
		}
		if !managedRegistryEndpointReady(ctx, scheme+"://"+registry.Endpoint) {
			status, detail = "degraded", "Registry 入口 /v2/ 未就绪"
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		detail = truncateManagedRegistryError(err)
	}
	now := time.Now()
	registry.Status, registry.LastError, registry.LastCheckedAt = status, detail, &now
	_ = h.store.UpdateManagedOCIRegistry(registry)
}
func managedRegistryEndpointReady(ctx context.Context, endpoint string) bool {
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
func truncateManagedRegistryError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 512 {
		return value[:512]
	}
	return value
}
