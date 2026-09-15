package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	security "github.com/cylism/cylism-manager/internal/security"
)

var (
	ErrManagedNodeRegistryMirror = errors.New("该镜像源由受管制品库维护，请在交付中心修改")
	ErrMirrorApplyRunning        = errors.New("该镜像源已有应用任务正在执行")
	ErrMirrorDisabled            = errors.New("镜像源已停用，不能应用")
)

const nodeRegistryMirrorApplyTimeout = 10 * time.Minute

// MirrorInput is the transport-independent definition of a K3s registry
// mirror. HTTP handlers map JSON requests into this type before calling the
// service, while other callers can reuse the same validation and lifecycle.
type MirrorInput struct {
	Name               string
	Registry           string
	Endpoints          []string
	VerificationImage  string
	Username           string
	Credential         string
	InsecureSkipVerify bool
	Enabled            *bool
}

// MirrorVerifier probes a mirror connection without exposing credentials to a
// caller. The service owns credential decryption and passes only the model.
type MirrorVerifier func(context.Context, *model.NodeRegistryMirror) error

// MirrorService owns persistence, validation, verification state and the
// single-flight application lifecycle for K3s node registry mirrors.
type MirrorService struct {
	repository repository.NodeRegistryMirrorRepository
	encKey     []byte
	applyMu    sync.Mutex
	running    map[uint]bool
}

func NewMirrorService(repository repository.NodeRegistryMirrorRepository, encKey []byte) *MirrorService {
	return &MirrorService{repository: repository, encKey: encKey, running: make(map[uint]bool)}
}

func (s *MirrorService) List() ([]model.NodeRegistryMirror, error) {
	mirrors, err := s.repository.ListNodeRegistryMirrors()
	if err != nil {
		return nil, err
	}
	for index := range mirrors {
		sanitizeMirror(&mirrors[index])
	}
	return mirrors, nil
}

func (s *MirrorService) Get(id uint) (*model.NodeRegistryMirror, error) {
	mirror, err := s.repository.GetNodeRegistryMirror(id)
	if err != nil {
		return nil, err
	}
	sanitizeMirror(mirror)
	return mirror, nil
}

func (s *MirrorService) Create(input MirrorInput, createdBy uint) (*model.NodeRegistryMirror, error) {
	mirror, err := s.build(input, nil)
	if err != nil {
		return nil, err
	}
	mirror.CreatedBy = createdBy
	if err := s.repository.CreateNodeRegistryMirror(mirror); err != nil {
		return nil, err
	}
	sanitizeMirror(mirror)
	return mirror, nil
}

func (s *MirrorService) Update(id uint, input MirrorInput) (*model.NodeRegistryMirror, error) {
	current, err := s.repository.GetNodeRegistryMirror(id)
	if err != nil {
		return nil, err
	}
	if current.ManagedRegistryID != nil {
		return nil, ErrManagedNodeRegistryMirror
	}
	mirror, err := s.build(input, current)
	if err != nil {
		return nil, err
	}
	if err := s.repository.UpdateNodeRegistryMirror(mirror); err != nil {
		return nil, err
	}
	sanitizeMirror(mirror)
	return mirror, nil
}

func (s *MirrorService) Delete(id uint) error {
	mirror, err := s.repository.GetNodeRegistryMirror(id)
	if err != nil {
		return err
	}
	if mirror.ManagedRegistryID != nil {
		return errors.New("该镜像源由受管制品库维护，不能单独删除")
	}
	return s.repository.DeleteNodeRegistryMirror(id)
}

func (s *MirrorService) Verify(ctx context.Context, id uint, verifier MirrorVerifier) (*model.NodeRegistryMirror, error) {
	mirror, err := s.repository.GetNodeRegistryMirror(id)
	if err != nil {
		return nil, err
	}
	status, detail := "succeeded", ""
	if !mirror.Enabled {
		status, detail = "failed", "节点镜像源已停用"
	} else if verifier == nil {
		status, detail = "failed", "镜像源连接检测不可用"
	} else if err := verifier(ctx, mirror); err != nil {
		status, detail = "failed", security.Truncate(strings.TrimSpace(err.Error()), 480)
	}
	if err := s.repository.UpdateNodeRegistryMirrorVerification(id, status, detail, time.Now()); err != nil {
		return nil, err
	}
	return s.Get(id)
}

// StartApply persists the pending state synchronously and launches the
// supplied infrastructure adapter in the background. It returns after the
// task is accepted, mirroring the existing REST contract.
func (s *MirrorService) StartApply(ctx context.Context, id uint, serverIDs []uint, apply NodeMirrorApplier) (*model.NodeRegistryMirror, error) {
	selected, err := s.repository.GetNodeRegistryMirror(id)
	if err != nil {
		return nil, err
	}
	if !selected.Enabled {
		return nil, ErrMirrorDisabled
	}
	if len(serverIDs) == 0 {
		return nil, errors.New("至少选择一个集群节点")
	}
	if apply == nil {
		return nil, errors.New("节点镜像源应用能力不可用")
	}
	content, err := s.renderK3sRegistries()
	if err != nil {
		return nil, err
	}
	servers, err := s.selectClusterServers(serverIDs)
	if err != nil {
		return nil, err
	}
	if !s.beginApply(id) {
		return nil, ErrMirrorApplyRunning
	}
	for _, server := range servers {
		if err := s.repository.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: id, ServerID: server.ID, Status: "pending", Detail: "等待应用"}); err != nil {
			s.finishApply(id)
			return nil, fmt.Errorf("初始化节点应用状态失败: %w", err)
		}
	}
	selected.LastAppliedAt = nil
	selected.LastApplyStatus = "applying"
	selected.LastApplyError = ""
	if err := s.repository.UpdateNodeRegistryMirror(selected); err != nil {
		s.finishApply(id)
		return nil, fmt.Errorf("保存应用任务状态失败: %w", err)
	}
	// Applying a mirror is asynchronous, so it must outlive the HTTP request
	// that accepted it. Keep request values while dropping request cancellation
	// and enforce a lifecycle owned by the background task instead.
	applyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nodeRegistryMirrorApplyTimeout)
	go func() {
		defer cancel()
		s.runApply(applyCtx, id, servers, content, apply)
	}()
	return s.Get(id)
}

func (s *MirrorService) build(input MirrorInput, current *model.NodeRegistryMirror) (*model.NodeRegistryMirror, error) {
	name, registry := strings.TrimSpace(input.Name), strings.TrimSpace(input.Registry)
	if name == "" || registry == "" {
		return nil, errors.New("名称和 Registry 地址必填")
	}
	if strings.Contains(registry, "://") || strings.Contains(registry, "/") {
		return nil, errors.New("Registry 地址仅支持主机名和可选端口")
	}
	endpoints := make([]string, 0, len(input.Endpoints))
	for _, raw := range input.Endpoints {
		endpoint := strings.TrimSpace(raw)
		if err := ValidateMirrorEndpointURL(endpoint); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	if len(endpoints) == 0 {
		return nil, errors.New("至少配置一个镜像地址")
	}
	verificationImage := strings.TrimSpace(input.VerificationImage)
	if verificationImage == "" && current != nil {
		verificationImage = current.VerificationImage
	}
	if err := ValidateVerificationImage(registry, verificationImage); err != nil {
		return nil, err
	}
	encodedEndpoints, _ := json.Marshal(endpoints)
	mirror := &model.NodeRegistryMirror{
		Name: name, Registry: registry, Endpoints: string(encodedEndpoints), VerificationImage: verificationImage,
		Username: strings.TrimSpace(input.Username), InsecureSkipVerify: input.InsecureSkipVerify, Enabled: true,
	}
	if current != nil {
		mirror.ID, mirror.CreatedAt, mirror.CreatedBy = current.ID, current.CreatedAt, current.CreatedBy
		mirror.Credential = current.Credential
		mirror.LastAppliedAt, mirror.LastApplyStatus, mirror.LastApplyError = current.LastAppliedAt, current.LastApplyStatus, current.LastApplyError
	}
	if input.Enabled != nil {
		mirror.Enabled = *input.Enabled
	}
	if input.Credential != "" {
		credential, err := EncryptCredential(s.encKey, input.Credential)
		if err != nil {
			return nil, errors.New("加密凭据失败")
		}
		mirror.Credential = credential
	}
	return mirror, nil
}

func (s *MirrorService) renderK3sRegistries() ([]byte, error) {
	mirrors, err := s.repository.ListNodeRegistryMirrors()
	if err != nil {
		return nil, err
	}
	return RenderK3sRegistriesWithStoredCredentials(mirrors, s.encKey)
}

func (s *MirrorService) selectClusterServers(serverIDs []uint) ([]model.Server, error) {
	servers, err := s.repository.ListServers()
	if err != nil {
		return nil, fmt.Errorf("读取集群节点失败: %w", err)
	}
	byID := make(map[uint]model.Server, len(servers))
	for _, server := range servers {
		if server.ClusterRole != "" {
			byID[server.ID] = server
		}
	}
	selected, seen := make([]model.Server, 0, len(serverIDs)), make(map[uint]struct{}, len(serverIDs))
	for _, serverID := range serverIDs {
		if _, duplicate := seen[serverID]; duplicate {
			continue
		}
		server, ok := byID[serverID]
		if !ok {
			return nil, fmt.Errorf("节点 %d 不是可应用的集群节点", serverID)
		}
		seen[serverID] = struct{}{}
		selected = append(selected, server)
	}
	return selected, nil
}

func (s *MirrorService) runApply(ctx context.Context, mirrorID uint, servers []model.Server, content []byte, apply NodeMirrorApplier) {
	successCount, failedCount := 0, 0
	for index := range servers {
		server := &servers[index]
		_ = s.repository.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: mirrorID, ServerID: server.ID, Status: "applying", Detail: "正在写入配置并重启 K3s"})
		status, detail := apply(ctx, server, content)
		if status == "success" {
			successCount++
		} else if status == "failed" {
			failedCount++
		}
		now := time.Now()
		_ = s.repository.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: mirrorID, ServerID: server.ID, Status: status, Detail: detail, AppliedAt: &now})
	}
	now := time.Now()
	if mirror, err := s.repository.GetNodeRegistryMirror(mirrorID); err == nil {
		mirror.LastAppliedAt = &now
		if failedCount == 0 && successCount == len(servers) {
			mirror.LastApplyStatus, mirror.LastApplyError = "succeeded", ""
		} else {
			mirror.LastApplyStatus = "failed"
			mirror.LastApplyError = fmt.Sprintf("%d 个节点成功，%d 个节点失败或跳过", successCount, len(servers)-successCount)
		}
		_ = s.repository.UpdateNodeRegistryMirror(mirror)
	}
	s.finishApply(mirrorID)
}

func (s *MirrorService) beginApply(id uint) bool {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	if s.running[id] {
		return false
	}
	s.running[id] = true
	return true
}

func (s *MirrorService) finishApply(id uint) {
	s.applyMu.Lock()
	delete(s.running, id)
	s.applyMu.Unlock()
}

func sanitizeMirror(mirror *model.NodeRegistryMirror) {
	mirror.CredentialConfigured = CredentialConfigured(mirror.Credential)
	mirror.Credential = ""
}
