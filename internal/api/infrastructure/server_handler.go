package infrastructure

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/http"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
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
	Name             string `json:"name" binding:"required"`
	Host             string `json:"host" binding:"required"`
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
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, err.Error())
		return
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
		Name: req.Name, Host: req.Host, SSHPort: req.SSHPort, SSHUser: req.SSHUser,
		SSHAuthType: req.SSHAuthType, SSHPassword: password, SSHKey: key,
		SSHKeyPassphrase: passphrase, SSHKeyHash: keyHash,
	}
	if err := h.cluster.CreateServer(server); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, err.Error())
		return
	}
	model.Success(c, server)
}

func (h *ServerHandler) List(c *gin.Context) {
	servers, err := h.cluster.ListServers()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.Success(c, servers)
}

func (h *ServerHandler) Get(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid server id")
		return
	}
	server, err := h.cluster.GetServer(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}
	model.Success(c, server)
}

func (h *ServerHandler) Update(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid server id")
		return
	}
	server, err := h.cluster.GetServer(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, err.Error())
		return
	}
	if value, ok := updates["name"].(string); ok {
		server.Name = value
	}
	if value, ok := updates["host"].(string); ok {
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
	if err := h.cluster.UpdateServer(server); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.Success(c, server)
}

func (h *ServerHandler) Delete(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid server id")
		return
	}
	if err := h.cluster.DeleteServer(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "操作成功")
}

func (h *ServerHandler) Unbind(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid server id")
		return
	}
	server, err := h.cluster.UnbindServer(id)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
			return
		}
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}
	model.SuccessWithMessage(c, server, "已解除集群绑定")
}

func (h *ServerHandler) Probe(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid id")
		return
	}
	result, err := h.cluster.ProbeServer(id)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
			return
		}
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}
	model.Success(c, result)
}

func (h *ServerHandler) Precheck(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid id")
		return
	}
	result, err := h.cluster.PrecheckServer(id)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
			return
		}
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}
	model.Success(c, result)
}

func (h *ServerHandler) Stats(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid id")
		return
	}
	result, err := h.cluster.ServerStats(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
			return
		}
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.Success(c, result)
}

func (h *ServerHandler) ResourceStats(c *gin.Context) {
	results, err := h.cluster.ResourceStats()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.Success(c, results)
}
