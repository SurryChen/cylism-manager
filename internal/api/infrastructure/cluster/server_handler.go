package cluster

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	crypto "github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ServerHandler maps server lifecycle HTTP requests onto the reusable Cluster
// Service. It deliberately contains no Kubernetes or SSH orchestration.
type ServerHandler struct {
	encKey  []byte
	cluster *cluster.Service
}

func NewServerHandler(encKey []byte, service *cluster.Service) *ServerHandler {
	return &ServerHandler{encKey: encKey, cluster: service}
}

type createServerRequest struct {
	Name             string `json:"name"`
	Host             string `json:"host" binding:"required"`
	Hostname         string `json:"hostname"`
	SSHPort          int    `json:"ssh_port"`
	SSHUser          string `json:"ssh_user"`
	SSHAuthType      string `json:"ssh_auth_type"`
	SSHPassword      string `json:"ssh_password"`
	SSHKey           string `json:"ssh_key"`
	SSHKeyPassphrase string `json:"ssh_key_passphrase"`
}

func (h *ServerHandler) Create(c *gin.Context) {
	var req createServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	req.Host = strings.TrimSpace(req.Host)
	if req.Host == "" {
		apiShared.BadRequest(c, "host is required")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = req.Host
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.SSHAuthType == "" {
		req.SSHAuthType = "password"
	}
	keyHash := ""
	if req.SSHKey != "" {
		hash := md5.Sum([]byte(req.SSHKey))
		keyHash = hex.EncodeToString(hash[:])
	}
	password, _ := crypto.Encrypt(h.encKey, req.SSHPassword)
	key, _ := crypto.Encrypt(h.encKey, req.SSHKey)
	passphrase, _ := crypto.Encrypt(h.encKey, req.SSHKeyPassphrase)
	server := &model.Server{
		Name: req.Name, Host: req.Host, Hostname: strings.TrimSpace(req.Hostname), SSHPort: req.SSHPort, SSHUser: req.SSHUser,
		SSHAuthType: req.SSHAuthType, SSHPassword: password, SSHKey: key,
		SSHKeyPassphrase: passphrase, SSHKeyHash: keyHash,
	}
	if err := h.cluster.CreateServer(server); err != nil {
		if existing, lookupErr := h.cluster.GetServerByHost(server.Host); lookupErr == nil && existing.ID != server.ID {
			apiShared.Conflict(c, "该主机已添加，请编辑已有服务器")
			return
		}
		apiShared.Conflict(c, err.Error())
		return
	}
	apiShared.Success(c, apiShared.ServerDTO(server))
}

func (h *ServerHandler) List(c *gin.Context) {
	servers, err := h.cluster.ListServersContext(c.Request.Context())
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.Success(c, apiShared.ServersDTO(servers))
}

func (h *ServerHandler) Get(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid server id")
		return
	}
	server, err := h.cluster.GetServer(id)
	if err != nil {
		apiShared.NotFound(c, "server not found")
		return
	}
	apiShared.Success(c, apiShared.ServerDTO(server))
}

func (h *ServerHandler) Update(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid server id")
		return
	}
	server, err := h.cluster.GetServer(id)
	if err != nil {
		apiShared.NotFound(c, "server not found")
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	if value, ok := updates["name"].(string); ok {
		server.Name = value
	}
	if value, ok := updates["host"].(string); ok {
		value = strings.TrimSpace(value)
		if value == "" {
			apiShared.BadRequest(c, "host is required")
			return
		}
		if _, named := updates["name"]; !named && server.Name == server.Host {
			server.Name = value
		}
		server.Host = value
	}
	if value, ok := updates["ssh_password"].(string); ok {
		server.SSHPassword, _ = crypto.Encrypt(h.encKey, value)
	}
	if value, ok := updates["ssh_key"].(string); ok {
		server.SSHKey, _ = crypto.Encrypt(h.encKey, value)
		if value == "" {
			server.SSHKeyHash = ""
		} else {
			hash := md5.Sum([]byte(value))
			server.SSHKeyHash = hex.EncodeToString(hash[:])
		}
	}
	if value, ok := updates["ssh_port"].(float64); ok {
		server.SSHPort = int(value)
	}
	if value, ok := updates["ssh_user"].(string); ok {
		server.SSHUser = value
	}
	if value, ok := updates["ssh_auth_type"].(string); ok {
		server.SSHAuthType = value
	}
	if value, ok := updates["hostname"].(string); ok {
		server.Hostname = strings.TrimSpace(value)
	}
	if err := h.cluster.UpdateServer(server); err != nil {
		if existing, lookupErr := h.cluster.GetServerByHost(server.Host); lookupErr == nil && existing.ID != server.ID {
			apiShared.Conflict(c, "该主机已添加，请编辑已有服务器")
			return
		}
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.Success(c, apiShared.ServerDTO(server))
}

func (h *ServerHandler) ResolveHostname(c *gin.Context) {
	server := &model.Server{}
	if rawID := c.Param("id"); rawID != "" {
		id, err := apiShared.ParsePositiveID(rawID)
		if err != nil {
			apiShared.BadRequest(c, "invalid server id")
			return
		}
		server, err = h.cluster.GetServer(id)
		if err != nil {
			apiShared.NotFound(c, "server not found")
			return
		}
	}
	var req createServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	if err := h.applyDraftConnection(server, req); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	hostname, err := h.cluster.ServerHostnameContext(c.Request.Context(), server)
	if err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"hostname": hostname})
}

func (h *ServerHandler) applyDraftConnection(server *model.Server, req createServerRequest) error {
	server.Host = strings.TrimSpace(req.Host)
	server.SSHPort = req.SSHPort
	server.SSHUser = req.SSHUser
	server.SSHAuthType = req.SSHAuthType
	if req.SSHPassword != "" {
		password, err := crypto.Encrypt(h.encKey, req.SSHPassword)
		if err != nil {
			return err
		}
		server.SSHPassword = password
	}
	if req.SSHKey != "" {
		key, err := crypto.Encrypt(h.encKey, req.SSHKey)
		if err != nil {
			return err
		}
		server.SSHKey = key
	}
	return nil
}

func (h *ServerHandler) Delete(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid server id")
		return
	}
	if err := h.cluster.DeleteServer(id); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.SuccessWithMessage(c, nil, "操作成功")
}

func (h *ServerHandler) Unbind(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid server id")
		return
	}
	server, err := h.cluster.UnbindServer(id)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.InternalError(c, err.Error())
			return
		}
		apiShared.NotFound(c, "server not found")
		return
	}
	apiShared.SuccessWithMessage(c, apiShared.ServerDTO(server), "已解除集群绑定")
}

func (h *ServerHandler) Probe(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid id")
		return
	}
	server, err := h.cluster.GetServer(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "server not found")
		} else {
			apiShared.InternalError(c, err.Error())
		}
		return
	}
	var req createServerRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		apiShared.BadRequest(c, err.Error())
		return
	}
	if req.Host != "" {
		if err := h.applyDraftConnection(server, req); err != nil {
			apiShared.InternalError(c, err.Error())
			return
		}
	}
	result, err := h.cluster.ProbeServerConfigContext(c.Request.Context(), server)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.InternalError(c, err.Error())
			return
		}
		apiShared.NotFound(c, "server not found")
		return
	}
	apiShared.Success(c, result)
}

func (h *ServerHandler) Precheck(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid id")
		return
	}
	result, err := h.cluster.PrecheckServerContext(c.Request.Context(), id)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.InternalError(c, err.Error())
			return
		}
		apiShared.NotFound(c, "server not found")
		return
	}
	apiShared.Success(c, result)
}

func (h *ServerHandler) Stats(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "invalid id")
		return
	}
	result, err := h.cluster.ServerStatsContext(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "server not found")
			return
		}
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

func (h *ServerHandler) ResourceStats(c *gin.Context) {
	results, err := h.cluster.ResourceStatsContext(c.Request.Context())
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	apiShared.Success(c, results)
}
