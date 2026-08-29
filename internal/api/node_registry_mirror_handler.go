package api

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type NodeRegistryMirrorHandler struct {
	store            *store.Store
	encKey           []byte
	verifyConnection nodeRegistryMirrorVerifier
	applyNode        registryservice.NodeMirrorApplier
	applyMu          sync.Mutex
	runningApplies   map[uint]bool
}
type nodeRegistryMirrorVerifier func(context.Context, *model.NodeRegistryMirror, []byte) error
type nodeRegistryMirrorApplyRequest struct {
	ServerIDs []uint `json:"server_ids"`
}
type nodeRegistryMirrorRequest struct {
	Name               string   `json:"name"`
	Registry           string   `json:"registry"`
	Endpoints          []string `json:"endpoints"`
	VerificationImage  string   `json:"verification_image"`
	Username           string   `json:"username"`
	Credential         string   `json:"credential"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify"`
	Enabled            *bool    `json:"enabled"`
}

func NewNodeRegistryMirrorHandler(s *store.Store, encKey []byte) *NodeRegistryMirrorHandler {
	return &NodeRegistryMirrorHandler{store: s, encKey: encKey, verifyConnection: verifyNodeRegistryMirrorConnection, applyNode: nil, runningApplies: make(map[uint]bool)}
}
func (h *NodeRegistryMirrorHandler) List(c *gin.Context) {
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取节点镜像源失败")
		return
	}
	model.Success(c, mirrors)
}
func (h *NodeRegistryMirrorHandler) Create(c *gin.Context) {
	var req nodeRegistryMirrorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像源定义无效")
		return
	}
	mirror, err := h.fromRequest(req, nil)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	mirror.CreatedBy = getUserID(c)
	if err := h.store.CreateNodeRegistryMirror(mirror); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "镜像源名称或 Registry 地址已存在")
		return
	}
	mirror.CredentialConfigured = registryservice.CredentialConfigured(mirror.Credential)
	model.Success(c, mirror)
}
func (h *NodeRegistryMirrorHandler) Update(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像源 ID 无效")
		return
	}
	current, err := h.store.GetNodeRegistryMirror(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像源不存在")
		return
	}
	if current.ManagedRegistryID != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "该镜像源由受管制品库维护，请在交付中心修改")
		return
	}
	var req nodeRegistryMirrorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像源定义无效")
		return
	}
	mirror, err := h.fromRequest(req, current)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.store.UpdateNodeRegistryMirror(mirror); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "镜像源名称或 Registry 地址已存在")
		return
	}
	mirror.CredentialConfigured = registryservice.CredentialConfigured(mirror.Credential)
	model.Success(c, mirror)
}
func (h *NodeRegistryMirrorHandler) Delete(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像源 ID 无效")
		return
	}
	mirror, err := h.store.GetNodeRegistryMirror(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像源不存在")
		return
	}
	if mirror.ManagedRegistryID != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "该镜像源由受管制品库维护，不能单独删除")
		return
	}
	if err := h.store.DeleteNodeRegistryMirror(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "删除节点镜像源失败")
		return
	}
	model.Success(c, gin.H{"id": id})
}

func (h *NodeRegistryMirrorHandler) Verify(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像源 ID 无效")
		return
	}
	mirror, err := h.store.GetNodeRegistryMirror(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像源不存在")
		return
	}

	status, detail := "succeeded", ""
	if !mirror.Enabled {
		status, detail = "failed", "节点镜像源已停用"
	} else if err := h.verifyConnection(c.Request.Context(), mirror, h.encKey); err != nil {
		status, detail = "failed", verificationDetail(err)
	}
	if err := h.store.UpdateNodeRegistryMirrorVerification(mirror.ID, status, detail, time.Now()); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存检测结果失败")
		return
	}
	mirror, err = h.store.GetNodeRegistryMirror(id)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取检测结果失败")
		return
	}
	model.Success(c, mirror)
}

func (h *NodeRegistryMirrorHandler) Apply(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像源 ID 无效")
		return
	}
	selected, err := h.store.GetNodeRegistryMirror(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像源不存在")
		return
	}
	if !selected.Enabled {
		model.Error(c, http.StatusConflict, model.CodeConflict, "镜像源已停用，不能应用")
		return
	}
	var request nodeRegistryMirrorApplyRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.ServerIDs) == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "至少选择一个集群节点")
		return
	}
	content, err := h.renderK3sRegistries()
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	servers, err := h.store.ListServers()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取集群节点失败")
		return
	}
	byID := make(map[uint]model.Server, len(servers))
	for index := range servers {
		if servers[index].ClusterRole != "" {
			byID[servers[index].ID] = servers[index]
		}
	}
	selectedServers := make([]model.Server, 0, len(request.ServerIDs))
	seen := make(map[uint]struct{}, len(request.ServerIDs))
	for _, serverID := range request.ServerIDs {
		if _, ok := seen[serverID]; ok {
			continue
		}
		server, ok := byID[serverID]
		if !ok {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, fmt.Sprintf("节点 %d 不是可应用的集群节点", serverID))
			return
		}
		seen[serverID] = struct{}{}
		selectedServers = append(selectedServers, server)
	}
	h.applyMu.Lock()
	if h.runningApplies[id] {
		h.applyMu.Unlock()
		model.Error(c, http.StatusConflict, model.CodeConflict, "该镜像源已有应用任务正在执行")
		return
	}
	h.runningApplies[id] = true
	h.applyMu.Unlock()

	for _, server := range selectedServers {
		if err := h.store.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: id, ServerID: server.ID, Status: "pending", Detail: "等待应用", AppliedAt: nil}); err != nil {
			h.finishApply(id)
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "初始化节点应用状态失败")
			return
		}
	}
	selected.LastAppliedAt = nil
	selected.LastApplyStatus = "applying"
	selected.LastApplyError = ""
	if err := h.store.UpdateNodeRegistryMirror(selected); err != nil {
		h.finishApply(id)
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存应用任务状态失败")
		return
	}
	go h.runApply(id, selectedServers, content)
	updated, _ := h.store.GetNodeRegistryMirror(id)
	model.SuccessWithMessage(c, updated, "应用任务已提交")
}

func (h *NodeRegistryMirrorHandler) ApplyStatus(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像源 ID 无效")
		return
	}
	mirror, err := h.store.GetNodeRegistryMirror(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像源不存在")
		return
	}
	model.Success(c, mirror)
}

func (h *NodeRegistryMirrorHandler) runApply(mirrorID uint, servers []model.Server, content []byte) {
	successCount := 0
	failedCount := 0
	for index := range servers {
		server := &servers[index]
		_ = h.store.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: mirrorID, ServerID: server.ID, Status: "applying", Detail: "正在写入配置并重启 K3s", AppliedAt: nil})
		apply := h.applyNode
		if apply == nil {
			apply = h.applyToNode
		}
		status, detail := apply(server, content)
		if status == "success" {
			successCount++
		} else if status == "failed" {
			failedCount++
		}
		now := time.Now()
		_ = h.store.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: mirrorID, ServerID: server.ID, Status: status, Detail: detail, AppliedAt: &now})
	}
	now := time.Now()
	if mirror, err := h.store.GetNodeRegistryMirror(mirrorID); err == nil {
		mirror.LastAppliedAt = &now
		if failedCount == 0 && successCount == len(servers) {
			mirror.LastApplyStatus = "succeeded"
			mirror.LastApplyError = ""
		} else {
			mirror.LastApplyStatus = "failed"
			mirror.LastApplyError = fmt.Sprintf("%d 个节点成功，%d 个节点失败或跳过", successCount, len(servers)-successCount)
		}
		_ = h.store.UpdateNodeRegistryMirror(mirror)
	}
	h.finishApply(mirrorID)
}

func (h *NodeRegistryMirrorHandler) finishApply(mirrorID uint) {
	h.applyMu.Lock()
	delete(h.runningApplies, mirrorID)
	h.applyMu.Unlock()
}

func (h *NodeRegistryMirrorHandler) fromRequest(req nodeRegistryMirrorRequest, current *model.NodeRegistryMirror) (*model.NodeRegistryMirror, error) {
	name, registry := strings.TrimSpace(req.Name), strings.TrimSpace(req.Registry)
	if name == "" || registry == "" {
		return nil, fmt.Errorf("名称和 Registry 地址必填")
	}
	if strings.Contains(registry, "://") || strings.Contains(registry, "/") {
		return nil, fmt.Errorf("Registry 地址仅支持主机名和可选端口")
	}
	endpoints := make([]string, 0, len(req.Endpoints))
	for _, raw := range req.Endpoints {
		value := strings.TrimSpace(raw)
		if err := registryservice.ValidateMirrorEndpointURL(value); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, value)
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("至少配置一个镜像地址")
	}
	verificationImage := strings.TrimSpace(req.VerificationImage)
	if verificationImage == "" && current != nil {
		verificationImage = current.VerificationImage
	}
	if err := registryservice.ValidateVerificationImage(registry, verificationImage); err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(endpoints)
	mirror := &model.NodeRegistryMirror{Name: name, Registry: registry, Endpoints: string(encoded), VerificationImage: verificationImage, Username: strings.TrimSpace(req.Username), InsecureSkipVerify: req.InsecureSkipVerify, Enabled: true}
	if current != nil {
		mirror.ID = current.ID
		mirror.CreatedAt = current.CreatedAt
		mirror.CreatedBy = current.CreatedBy
		mirror.Credential = current.Credential
		mirror.LastAppliedAt = current.LastAppliedAt
		mirror.LastApplyStatus = current.LastApplyStatus
		mirror.LastApplyError = current.LastApplyError
	}
	if req.Enabled != nil {
		mirror.Enabled = *req.Enabled
	}
	if req.Credential != "" {
		credential, err := registryservice.EncryptCredential(h.encKey, req.Credential)
		if err != nil {
			return nil, fmt.Errorf("加密凭据失败")
		}
		mirror.Credential = credential
	}
	return mirror, nil
}

func verifyNodeRegistryMirrorConnection(ctx context.Context, mirror *model.NodeRegistryMirror, encKey []byte) error {
	if mirror.VerificationImage == "" {
		return fmt.Errorf("请先配置验证镜像")
	}
	var endpoints []string
	if err := json.Unmarshal([]byte(mirror.Endpoints), &endpoints); err != nil {
		return fmt.Errorf("镜像地址配置损坏")
	}
	var failures []string
	for _, endpoint := range endpoints {
		ref, err := registryservice.MirrorVerificationReference(mirror.Registry, mirror.VerificationImage, endpoint)
		if err == nil {
			err = verifyNodeRegistryMirrorEndpoint(ctx, mirror, ref, encKey)
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", endpoint, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func verifyNodeRegistryMirrorEndpoint(ctx context.Context, mirror *model.NodeRegistryMirror, ref name.Reference, encKey []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	options := []remote.Option{remote.WithContext(ctx), remote.WithAuth(authn.Anonymous)}
	if mirror.Username != "" {
		if mirror.Credential == "" {
			return fmt.Errorf("已配置账号但缺少密码或 Token")
		}
		credential, err := registryservice.DecryptCredential(encKey, mirror.Credential)
		if err != nil {
			return fmt.Errorf("读取镜像源凭据失败")
		}
		options[1] = remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: mirror.Username, Password: credential}))
	}
	if mirror.InsecureSkipVerify {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		options = append(options, remote.WithTransport(transport))
	}
	if _, err := remote.Head(ref, options...); err != nil {
		return fmt.Errorf("无法访问验证镜像: %w", err)
	}
	return nil
}
func (h *NodeRegistryMirrorHandler) renderK3sRegistries() ([]byte, error) {
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		return nil, err
	}
	return registryservice.RenderK3sRegistriesWithStoredCredentials(mirrors, h.encKey)
}
func (h *NodeRegistryMirrorHandler) applyToNode(server *model.Server, content []byte) (string, string) {
	return applyK3sRegistriesToNode(server, h.encKey, content)
}

// applyK3sRegistriesToNode is the SSH adapter shared by Registry workflows.
// The caller supplies already-rendered configuration, so this adapter only
// performs the guarded remote write and K3s restart scheduling.
func applyK3sRegistriesToNode(server *model.Server, encKey, content []byte) (string, string) {
	if server.SSHAuthType != "key" || server.SSHKey == "" {
		return "skipped", "需要已配置的 SSH 密钥认证"
	}
	payload := base64.StdEncoding.EncodeToString(content)
	command := nodeRegistryMirrorApplyCommand(payload)
	out, err := sshExec(90*time.Second, append(buildSSHArgs(server, encKey, server.Host), command))
	if err != nil {
		return "failed", strings.TrimSpace(string(out))
	}
	return "success", "已备份旧配置，K3s 服务重启已安排"
}

func nodeRegistryMirrorApplyCommand(payload string) string {
	return fmt.Sprintf("set -eu; sudo -n mkdir -p /etc/rancher/k3s; if sudo -n test -f /etc/rancher/k3s/registries.yaml; then sudo -n cp /etc/rancher/k3s/registries.yaml /etc/rancher/k3s/registries.yaml.cylism-backup; fi; printf %%s %s | base64 -d | sudo -n tee /etc/rancher/k3s/registries.yaml.tmp >/dev/null; sudo -n chmod 600 /etc/rancher/k3s/registries.yaml.tmp; sudo -n mv /etc/rancher/k3s/registries.yaml.tmp /etc/rancher/k3s/registries.yaml; if sudo -n systemctl is-active --quiet k3s; then service=k3s; elif sudo -n systemctl is-active --quiet k3s-agent; then service=k3s-agent; else echo '未检测到 k3s 或 k3s-agent 服务' >&2; exit 1; fi; sudo -n true; nohup sudo -n sh -c \"sleep 2; systemctl restart $service\" </dev/null >/dev/null 2>&1 &", payload)
}
