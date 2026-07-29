package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"
)

type NodeRegistryMirrorHandler struct {
	store  *store.Store
	encKey []byte
}
type nodeRegistryMirrorRequest struct {
	Name               string   `json:"name"`
	Registry           string   `json:"registry"`
	Endpoints          []string `json:"endpoints"`
	Username           string   `json:"username"`
	Credential         string   `json:"credential"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify"`
	Enabled            *bool    `json:"enabled"`
}

func NewNodeRegistryMirrorHandler(s *store.Store, encKey []byte) *NodeRegistryMirrorHandler {
	return &NodeRegistryMirrorHandler{store: s, encKey: encKey}
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
	mirror.CredentialConfigured = mirror.Credential != ""
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
	mirror.CredentialConfigured = mirror.Credential != ""
	model.Success(c, mirror)
}
func (h *NodeRegistryMirrorHandler) Delete(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像源 ID 无效")
		return
	}
	if err := h.store.DeleteNodeRegistryMirror(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "删除节点镜像源失败")
		return
	}
	model.Success(c, gin.H{"id": id})
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
	results := make([]gin.H, 0)
	now := time.Now()
	successCount := 0
	for index := range servers {
		server := &servers[index]
		if server.ClusterRole == "" {
			continue
		}
		status, detail := h.applyToNode(server, content)
		if status == "success" {
			successCount++
		}
		appliedAt := now
		_ = h.store.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: id, ServerID: server.ID, Status: status, Detail: detail, AppliedAt: &appliedAt})
		results = append(results, gin.H{"server_id": server.ID, "server_name": server.Name, "status": status, "detail": detail})
	}
	selected.LastAppliedAt = &now
	if successCount == len(results) && len(results) > 0 {
		selected.LastApplyStatus = "succeeded"
		selected.LastApplyError = ""
	} else {
		selected.LastApplyStatus = "failed"
		selected.LastApplyError = "部分节点下发失败"
	}
	_ = h.store.UpdateNodeRegistryMirror(selected)
	model.Success(c, gin.H{"results": results})
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
		parsed, err := url.ParseRequestURI(value)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return nil, fmt.Errorf("镜像地址必须是 HTTP 或 HTTPS URL")
		}
		endpoints = append(endpoints, value)
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("至少配置一个镜像地址")
	}
	encoded, _ := json.Marshal(endpoints)
	mirror := &model.NodeRegistryMirror{Name: name, Registry: registry, Endpoints: string(encoded), Username: strings.TrimSpace(req.Username), InsecureSkipVerify: req.InsecureSkipVerify, Enabled: true}
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
		credential, err := crypto.Encrypt(h.encKey, req.Credential)
		if err != nil {
			return nil, fmt.Errorf("加密凭据失败")
		}
		mirror.Credential = credential
	}
	return mirror, nil
}
func (h *NodeRegistryMirrorHandler) renderK3sRegistries() ([]byte, error) {
	mirrors, err := h.store.ListNodeRegistryMirrors()
	if err != nil {
		return nil, err
	}
	configs := map[string]interface{}{}
	registryMirrors := map[string]interface{}{}
	for _, mirror := range mirrors {
		if !mirror.Enabled {
			continue
		}
		var endpoints []string
		if json.Unmarshal([]byte(mirror.Endpoints), &endpoints) != nil {
			return nil, fmt.Errorf("镜像源 %q 配置损坏", mirror.Name)
		}
		registryMirrors[mirror.Registry] = map[string]interface{}{"endpoint": endpoints}
		if mirror.Username != "" || mirror.Credential != "" || mirror.InsecureSkipVerify {
			config := map[string]interface{}{}
			if mirror.Username != "" {
				credential, err := crypto.Decrypt(h.encKey, mirror.Credential)
				if err != nil {
					return nil, fmt.Errorf("解密镜像源 %q 凭据失败", mirror.Name)
				}
				config["auth"] = map[string]string{"username": mirror.Username, "password": credential}
			}
			if mirror.InsecureSkipVerify {
				config["tls"] = map[string]bool{"insecure_skip_verify": true}
			}
			configs[mirror.Registry] = config
		}
	}
	if len(registryMirrors) == 0 {
		return nil, fmt.Errorf("没有已启用的节点镜像源")
	}
	return yaml.Marshal(map[string]interface{}{"mirrors": registryMirrors, "configs": configs})
}
func (h *NodeRegistryMirrorHandler) applyToNode(server *model.Server, content []byte) (string, string) {
	if server.SSHAuthType != "key" || server.SSHKey == "" {
		return "skipped", "需要已配置的 SSH 密钥认证"
	}
	payload := base64.StdEncoding.EncodeToString(content)
	command := fmt.Sprintf("set -eu; sudo -n mkdir -p /etc/rancher/k3s; if sudo -n test -f /etc/rancher/k3s/registries.yaml; then sudo -n cp /etc/rancher/k3s/registries.yaml /etc/rancher/k3s/registries.yaml.cylism-backup; fi; printf %%s %s | base64 -d | sudo -n tee /etc/rancher/k3s/registries.yaml.tmp >/dev/null; sudo -n chmod 600 /etc/rancher/k3s/registries.yaml.tmp; sudo -n mv /etc/rancher/k3s/registries.yaml.tmp /etc/rancher/k3s/registries.yaml; if sudo -n systemctl is-active --quiet k3s; then sudo -n systemctl restart k3s; elif sudo -n systemctl is-active --quiet k3s-agent; then sudo -n systemctl restart k3s-agent; else echo '未检测到 k3s 或 k3s-agent 服务' >&2; exit 1; fi", payload)
	out, err := sshExec(90*time.Second, append(buildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return "failed", strings.TrimSpace(string(out))
	}
	return "success", "已备份旧配置并重启 K3s 服务"
}
