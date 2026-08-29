package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/google/go-containerregistry/pkg/name"
)

var (
	ErrReleaseTemplateRead = errors.New("release template read")
	ErrReleaseSecretRead   = errors.New("release secret read")
)

// PreparedRelease is the resolved runtime form of a release. Secrets and
// registry credentials are present only in Spec and are never persisted in
// Release.DesiredSpec.
type PreparedRelease struct {
	Release *model.Release
	Spec    ReleaseSpec
	Service *Service
}

// ReleaseWorkflow owns the application release use cases that used to be
// assembled in HTTP handlers.
type ReleaseWorkflow struct {
	store   *store.Store
	encKey  []byte
	applier ResourceApplier
}

type endpointSynchronizer interface {
	SyncApplicationEndpoints(ctx context.Context, application ApplicationContext, endpoints []model.ApplicationEndpoint, servicePort int32) error
}

func NewReleaseWorkflow(st *store.Store, encKey []byte, applier ResourceApplier) *ReleaseWorkflow {
	return &ReleaseWorkflow{store: st, encKey: append([]byte(nil), encKey...), applier: applier}
}

func (w *ReleaseWorkflow) CreateFromTemplate(ctx context.Context, app *model.Application, template *model.ApplicationDeploymentTemplate, version string, userID uint) (*PreparedRelease, error) {
	if app == nil || template == nil || !template.Enabled {
		return nil, fmt.Errorf("上线模板不存在或已停用")
	}
	var spec ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReleaseTemplateRead, err)
	}
	secrets, err := w.decryptTemplateSecrets(template.EncryptedSecrets)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReleaseSecretRead, err)
	}
	if len(spec.Secrets) > 0 && len(secrets) == 0 {
		return nil, fmt.Errorf("模板中的 Secret 尚未配置明文值")
	}
	spec.Secrets = secrets
	spec.Version = strings.TrimSpace(version)
	spec.Image, err = imageWithVersion(spec.Image, spec.Version)
	if err != nil {
		return nil, err
	}
	if err := w.PrepareRuntimeSpec(app, &spec); err != nil {
		return nil, err
	}
	spec.Endpoint = EndpointSpec{Exposure: ExposureCluster}
	return w.create(ctx, app, template, userID, spec)
}

func (w *ReleaseWorkflow) Restart(ctx context.Context, app *model.Application, userID uint) (*PreparedRelease, error) {
	template, err := w.store.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil || !template.Enabled {
		return nil, fmt.Errorf("应用没有可用的默认上线模板")
	}
	active, err := w.store.GetLatestSuccessfulRelease(app.ID)
	if err != nil {
		return nil, fmt.Errorf("应用尚无可重启的成功发布")
	}
	var spec ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, fmt.Errorf("读取上线模板失败")
	}
	secrets, err := w.decryptTemplateSecrets(template.EncryptedSecrets)
	if err != nil {
		return nil, fmt.Errorf("读取模板 Secret 失败")
	}
	spec.Secrets = secrets
	spec.Image, spec.Version = active.Image, active.Version
	if err := w.PrepareRuntimeSpec(app, &spec); err != nil {
		return nil, err
	}
	spec.Endpoint = EndpointSpec{Exposure: ExposureCluster}
	return w.create(ctx, app, template, userID, spec)
}

func (w *ReleaseWorkflow) Retry(releaseID, userID uint) (*PreparedRelease, error) {
	service := NewService(w.store, w.applier)
	release, spec, err := service.RetryRelease(releaseID, userID)
	if err != nil {
		return nil, err
	}
	app, err := w.store.GetApplication(release.ApplicationID)
	if err != nil {
		return nil, err
	}
	if err := w.PrepareRuntimeSpec(app, &spec); err != nil {
		return nil, err
	}
	spec.Endpoint = EndpointSpec{Exposure: ExposureCluster}
	return &PreparedRelease{Release: release, Spec: spec, Service: service}, nil
}

func (w *ReleaseWorkflow) Rollback(releaseID, userID uint) (*PreparedRelease, error) {
	service := NewService(w.store, w.applier)
	release, spec, err := service.RollbackRelease(releaseID, userID)
	if err != nil {
		return nil, err
	}
	app, err := w.store.GetApplication(release.ApplicationID)
	if err != nil {
		return nil, err
	}
	if err := w.PrepareRuntimeSpec(app, &spec); err != nil {
		return nil, err
	}
	spec.Endpoint = EndpointSpec{Exposure: ExposureCluster}
	return &PreparedRelease{Release: release, Spec: spec, Service: service}, nil
}

// ExecuteAsync runs the Kubernetes lifecycle after the HTTP request has
// returned. The release record remains the durable source of progress.
func (w *ReleaseWorkflow) ExecuteAsync(app *model.Application, prepared *PreparedRelease) {
	if app == nil || prepared == nil || prepared.Release == nil || prepared.Service == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		_ = prepared.Service.ExecuteReleaseWithPostApply(ctx, prepared.Release.ID, app, prepared.Spec, func() error {
			return w.syncApplicationEndpoints(ctx, app, prepared.Spec.Service)
		})
	}()
}

// SyncApplicationEndpoints reconciles the current application bindings with a
// rendered Service. It is also used by endpoint CRUD callers after changing a
// binding outside the release lifecycle.
func (w *ReleaseWorkflow) SyncApplicationEndpoints(ctx context.Context, app *model.Application, service ServiceSpec) error {
	return w.syncApplicationEndpoints(ctx, app, service)
}

// SyncManagedFiles persists the ConfigMap and Secret keys that integrations
// may edit. The template remains the source of desired content.
func (w *ReleaseWorkflow) SyncManagedFiles(app *model.Application, spec ReleaseSpec, userID uint) error {
	NormalizeManagedKeys(&spec)
	bindings := make(map[string]struct{})
	for _, managed := range []struct {
		keys   []string
		kind   string
		name   string
		secret bool
	}{
		{keys: spec.ConfigManagedKeys, kind: FileMountSourceConfigMap, name: app.Name + "-config"},
		{keys: spec.SecretManagedKeys, kind: FileMountSourceSecret, name: app.Name + "-secret", secret: true},
	} {
		for _, key := range managed.keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			bindings[managedFileBinding(managed.kind, managed.name, key)] = struct{}{}
			file := &model.ApplicationManagedFile{
				ApplicationID: app.ID, ResourceKind: managed.kind, ResourceName: managed.name, Key: key,
				MountPath: managedMountPath(spec, managed.secret, key), Format: "text", CreatedBy: userID, Enabled: true,
			}
			if err := w.store.UpsertApplicationManagedFile(file); err != nil {
				return err
			}
		}
	}
	return w.store.DisableApplicationManagedFilesNotIn(app.ID, bindings)
}

func (w *ReleaseWorkflow) create(ctx context.Context, app *model.Application, template *model.ApplicationDeploymentTemplate, userID uint, spec ReleaseSpec) (*PreparedRelease, error) {
	service := NewService(w.store, w.applier)
	release, err := service.CreateRelease(ctx, app.ID, userID, spec)
	if err != nil {
		return nil, err
	}
	templateID := template.ID
	release.TemplateID, release.TemplateRevision = &templateID, template.Revision
	if err := w.store.UpdateRelease(release); err != nil {
		return nil, err
	}
	return &PreparedRelease{Release: release, Spec: spec, Service: service}, nil
}

func (w *ReleaseWorkflow) PrepareRuntimeSpec(app *model.Application, spec *ReleaseSpec) error {
	if spec == nil {
		return fmt.Errorf("发布定义无效")
	}
	if err := w.PrepareRegistrySpec(app, spec); err != nil {
		return err
	}
	return w.prepareMirrorSpec(spec)
}

// PrepareRegistrySpec applies the selected project registry and resolves its
// runtime-only credential. Template editing uses this to persist a canonical
// image name without triggering node-level verification selection.
func (w *ReleaseWorkflow) PrepareRegistrySpec(app *model.Application, spec *ReleaseSpec) error {
	if spec == nil {
		return fmt.Errorf("发布定义无效")
	}
	if spec.RegistryID != 0 {
		if app == nil {
			return fmt.Errorf("应用不存在")
		}
		registry, err := w.store.GetImageRegistryForProject(spec.RegistryID, app.ProjectID)
		if err != nil || !registry.Enabled {
			return fmt.Errorf("镜像仓库不存在、未授权当前项目或已禁用")
		}
		image, err := registryImageReference(registry.Endpoint, spec.Image)
		if err != nil {
			return err
		}
		spec.Image, spec.RegistryEndpoint, spec.RegistryAuthType = image, registry.Endpoint, registry.AuthType
		if registry.AuthType != registryservice.AuthTypeAnonymous {
			credential, err := registryservice.DecryptCredential(w.encKey, registry.Credential)
			if err != nil {
				return fmt.Errorf("读取镜像仓库凭据失败")
			}
			spec.RegistryUsername = registry.Username
			if registry.AuthType == registryservice.AuthTypeToken && spec.RegistryUsername == "" {
				spec.RegistryUsername = "token"
			}
			spec.RegistryCredential = credential
		}
	}
	return nil
}

func (w *ReleaseWorkflow) prepareMirrorSpec(spec *ReleaseSpec) error {
	spec.ImageVerificationEndpoint = ""
	spec.ImageVerificationUsername = ""
	spec.ImageVerificationCredential = ""
	spec.ImageVerificationInsecureSkipVerify = false
	if strings.TrimSpace(spec.NodeName) == "" {
		return nil
	}
	ref, err := parseImageReference(spec.Image)
	if err != nil {
		return fmt.Errorf("镜像地址无效: %w", err)
	}
	mirrors, err := w.store.ListNodeRegistryMirrors()
	if err != nil {
		return fmt.Errorf("读取节点镜像源失败: %w", err)
	}
	for _, mirror := range mirrors {
		if !mirror.Enabled || !registryservice.SameRegistry(mirror.Registry, ref) || !mirrorAppliedToNode(mirror, spec.NodeName) {
			continue
		}
		var endpoints []string
		if err := json.Unmarshal([]byte(mirror.Endpoints), &endpoints); err != nil || len(endpoints) == 0 {
			return fmt.Errorf("节点镜像源 %q 配置损坏", mirror.Name)
		}
		endpoint := strings.TrimSpace(endpoints[0])
		if endpoint == "" {
			return fmt.Errorf("节点镜像源 %q 未配置可用地址", mirror.Name)
		}
		spec.ImageVerificationEndpoint, spec.ImageVerificationUsername = endpoint, mirror.Username
		spec.ImageVerificationInsecureSkipVerify = mirror.InsecureSkipVerify
		if mirror.Username != "" {
			credential, err := registryservice.DecryptCredential(w.encKey, mirror.Credential)
			if err != nil {
				return fmt.Errorf("读取节点镜像源 %q 凭据失败", mirror.Name)
			}
			spec.ImageVerificationCredential = credential
		}
		return nil
	}
	return nil
}

func (w *ReleaseWorkflow) decryptTemplateSecrets(encrypted string) (map[string]string, error) {
	if strings.TrimSpace(encrypted) == "" {
		return map[string]string{}, nil
	}
	if len(w.encKey) == 0 {
		return nil, fmt.Errorf("平台加密密钥未配置")
	}
	plaintext, err := registryservice.DecryptCredential(w.encKey, encrypted)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	if err := json.Unmarshal([]byte(plaintext), &values); err != nil {
		return nil, err
	}
	return values, nil
}

var releaseImageTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

func imageWithVersion(repository, version string) (string, error) {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	if repository == "" || strings.ContainsAny(repository, " \t\r\n") || strings.Contains(repository, "@") {
		return "", fmt.Errorf("镜像路径无效")
	}
	if strings.Contains(repository[strings.LastIndex(repository, "/")+1:], ":") {
		return "", fmt.Errorf("上线模板镜像路径不应包含 Tag，请在发布时填写版本号")
	}
	version = strings.TrimSpace(version)
	if !releaseImageTagPattern.MatchString(version) {
		return "", fmt.Errorf("版本号必须是合法的镜像 Tag")
	}
	return repository + ":" + version, nil
}

func registryImageReference(endpoint, image string) (string, error) {
	image = strings.Trim(strings.TrimSpace(image), "/")
	if image == "" || strings.ContainsAny(image, " \t\r\n") {
		return "", fmt.Errorf("镜像路径不能为空且不能包含空格")
	}
	prefix := strings.Trim(strings.TrimSpace(endpoint), "/") + "/"
	if strings.HasPrefix(image, prefix) {
		return image, nil
	}
	return prefix + image, nil
}

func parseImageReference(image string) (string, error) {
	ref, err := name.ParseReference(strings.TrimSpace(image))
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}

func mirrorAppliedToNode(mirror model.NodeRegistryMirror, nodeName string) bool {
	for _, status := range mirror.NodeStatuses {
		if status.Status == "success" && status.Server.K8sNodeName == nodeName {
			return true
		}
	}
	return false
}

func (w *ReleaseWorkflow) syncApplicationEndpoints(ctx context.Context, app *model.Application, service ServiceSpec) error {
	synchronizer, ok := w.applier.(endpointSynchronizer)
	if !ok {
		return fmt.Errorf("Kubernetes 入口同步器未初始化")
	}
	endpoints, err := w.store.ListApplicationEndpoints(app.ID)
	if err != nil {
		return fmt.Errorf("读取应用入口: %w", err)
	}
	primaryTCPPort, hasTCPPort := service.PrimaryTCPPort()
	context := applicationContextFor(app)
	if !hasTCPPort {
		servicePorts := service.PortSpecs()
		if len(servicePorts) == 0 {
			return fmt.Errorf("应用没有可绑定的 Service 端口")
		}
		if err := synchronizer.SyncApplicationEndpoints(ctx, context, nil, servicePorts[0].Port); err != nil {
			return fmt.Errorf("移除 UDP Service 的 HTTP Ingress: %w", err)
		}
		return nil
	}
	for index := range endpoints {
		if endpoints[index].Protocol == ServiceProtocolTCP && !servicePortExists(service, endpoints[index].ServicePort, ServiceProtocolTCP) {
			endpoints[index].ServicePort = primaryTCPPort.Port
			if err := w.store.UpdateApplicationEndpoint(&endpoints[index]); err != nil {
				return fmt.Errorf("更新应用入口端口: %w", err)
			}
		}
		if endpoints[index].Protocol == "" {
			resolved, err := ResolveEndpointServicePort(service, endpoints[index].ServicePort, "", endpointUsesIngress(endpoints[index]))
			if err != nil {
				return fmt.Errorf("校验应用入口端口: %w", err)
			}
			endpoints[index].ServicePort = resolved.Port
			endpoints[index].Protocol = resolved.Protocol
			if err := w.store.UpdateApplicationEndpoint(&endpoints[index]); err != nil {
				return fmt.Errorf("更新应用入口协议: %w", err)
			}
		}
		if endpointUsesIngress(endpoints[index]) && strings.ToUpper(endpoints[index].Protocol) != ServiceProtocolTCP {
			return fmt.Errorf("入口 %s 使用 UDP 端口但启用了 HTTP Ingress", endpoints[index].Domain)
		}
	}
	if err := synchronizer.SyncApplicationEndpoints(ctx, context, endpoints, primaryTCPPort.Port); err != nil {
		return fmt.Errorf("同步应用入口: %w", err)
	}
	return nil
}

// ResolveEndpointServicePort verifies an endpoint's requested Service port.
// Empty values retain the historical preference for the primary TCP port.
func ResolveEndpointServicePort(spec ServiceSpec, requestedPort int32, requestedProtocol string, ingress bool) (ServicePortSpec, error) {
	ports := spec.PortSpecs()
	if len(ports) == 0 {
		return ServicePortSpec{}, fmt.Errorf("应用没有可绑定的 Service 端口")
	}
	protocol := strings.ToUpper(strings.TrimSpace(requestedProtocol))
	if protocol != "" && protocol != ServiceProtocolTCP && protocol != ServiceProtocolUDP {
		return ServicePortSpec{}, fmt.Errorf("Service 协议必须为 TCP 或 UDP")
	}
	if requestedPort == 0 {
		if primary, ok := spec.PrimaryTCPPort(); ok {
			requestedPort = primary.Port
			if protocol == "" {
				protocol = ServiceProtocolTCP
			}
		} else {
			requestedPort = ports[0].Port
		}
	}
	for _, port := range ports {
		if port.Port != requestedPort {
			continue
		}
		actualProtocol := strings.ToUpper(strings.TrimSpace(port.Protocol))
		if actualProtocol == "" {
			actualProtocol = ServiceProtocolTCP
		}
		if protocol != "" && protocol != actualProtocol {
			return ServicePortSpec{}, fmt.Errorf("Service 端口 %d 的协议为 %s，不是 %s", requestedPort, actualProtocol, protocol)
		}
		if ingress && actualProtocol != ServiceProtocolTCP {
			return ServicePortSpec{}, fmt.Errorf("UDP Service 不支持 HTTP Ingress 域名绑定")
		}
		port.Protocol = actualProtocol
		return port, nil
	}
	return ServicePortSpec{}, fmt.Errorf("Service 端口 %d 不存在，请选择模板中已声明的端口", requestedPort)
}

func applicationContextFor(app *model.Application) ApplicationContext {
	return ApplicationContext{
		ProjectID: app.ProjectID, EnvironmentID: app.EnvironmentID,
		ProjectName: app.Project.Name, EnvironmentName: app.Environment.Name,
		ApplicationName: app.Name, Namespace: app.Environment.Namespace, WorkloadKind: app.WorkloadKind,
	}
}

func endpointUsesIngress(endpoint model.ApplicationEndpoint) bool {
	return strings.ToLower(strings.TrimSpace(endpoint.IngressMode)) != "metadata"
}

func servicePortExists(spec ServiceSpec, requestedPort int32, requestedProtocol string) bool {
	for _, port := range spec.PortSpecs() {
		protocol := strings.ToUpper(strings.TrimSpace(port.Protocol))
		if protocol == "" {
			protocol = ServiceProtocolTCP
		}
		if port.Port == requestedPort && protocol == requestedProtocol {
			return true
		}
	}
	return false
}

func managedMountPath(spec ReleaseSpec, secret bool, key string) string {
	wantSource := FileMountSourceApplicationConfig
	if secret {
		wantSource = FileMountSourceApplicationSecret
	}
	for _, mount := range spec.FileMounts {
		if mount.SourceType == wantSource && mount.Key == key {
			return mount.MountPath
		}
	}
	return ""
}

func managedFileBinding(kind, name, key string) string {
	return strings.Join([]string{kind, name, key}, "\x00")
}
