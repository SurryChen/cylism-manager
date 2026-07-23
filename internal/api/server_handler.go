package api

import (
	"net/http"
	"strconv"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type ServerHandler struct {
	store     *store.Store
	encKey    []byte
	deploySvc DeployService
}

type DeployService interface {
	DeployAgent(server *model.Server) error
	TestSSH(server *model.Server) (string, error)
}

func NewServerHandler(s *store.Store, encKey []byte, deploySvc DeployService) *ServerHandler {
	return &ServerHandler{store: s, encKey: encKey, deploySvc: deploySvc}
}

type createServerReq struct {
	Name             string `json:"name" binding:"required"`
	Host             string `json:"host" binding:"required"`
	Port             int    `json:"port"`
	SSHHost          string `json:"ssh_host"`
	SSHPort          int    `json:"ssh_port"`
	SSHUser          string `json:"ssh_user"`
	SSHAuthType      string `json:"ssh_auth_type"`
	SSHPassword      string `json:"ssh_password"`
	SSHKey           string `json:"ssh_key"`
	SSHKeyPassphrase string `json:"ssh_key_passphrase"`
}

func (h *ServerHandler) Create(c *gin.Context) {
	var req createServerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 { req.Port = 9527 }
	if req.SSHPort == 0 { req.SSHPort = 22 }
	if req.SSHAuthType == "" { req.SSHAuthType = "password" }

	encPassword, _ := crypto.Encrypt(h.encKey, req.SSHPassword)
	encKey, _ := crypto.Encrypt(h.encKey, req.SSHKey)
	encPassphrase, _ := crypto.Encrypt(h.encKey, req.SSHKeyPassphrase)

	server := &model.Server{
		Name: req.Name, Host: req.Host, Port: req.Port,
		SSHHost: req.SSHHost, SSHPort: req.SSHPort, SSHUser: req.SSHUser,
		SSHAuthType: req.SSHAuthType, SSHPassword: encPassword,
		SSHKey: encKey, SSHKeyPassphrase: encPassphrase, Status: "offline",
	}
	if req.SSHHost == "" { server.SSHHost = req.Host }

	if err := h.store.CreateServer(server); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, server)
}

func (h *ServerHandler) List(c *gin.Context) {
	servers, err := h.store.ListServers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, servers)
}

func (h *ServerHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil { c.JSON(http.StatusNotFound, gin.H{"error": "server not found"}); return }
	c.JSON(http.StatusOK, server)
}

func (h *ServerHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil { c.JSON(http.StatusNotFound, gin.H{"error": "server not found"}); return }

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return
	}
	if v, ok := updates["name"]; ok { server.Name = v.(string) }
	if v, ok := updates["port"]; ok { server.Port = int(v.(float64)) }
	if v, ok := updates["ssh_password"]; ok { enc, _ := crypto.Encrypt(h.encKey, v.(string)); server.SSHPassword = enc }
	if v, ok := updates["ssh_key"]; ok { enc, _ := crypto.Encrypt(h.encKey, v.(string)); server.SSHKey = enc }
	if v, ok := updates["ssh_host"]; ok { server.SSHHost = v.(string) }
	if v, ok := updates["ssh_port"]; ok { server.SSHPort = int(v.(float64)) }
	if v, ok := updates["ssh_user"]; ok { server.SSHUser = v.(string) }
	if v, ok := updates["ssh_auth_type"]; ok { server.SSHAuthType = v.(string) }

	if err := h.store.UpdateServer(server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return
	}
	c.JSON(http.StatusOK, server)
}

func (h *ServerHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.store.DeleteServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ServerHandler) Deploy(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil { c.JSON(http.StatusNotFound, gin.H{"error": "server not found"}); return }

	server.Status = "deploying"
	h.store.UpdateServer(server)

	if err := h.deploySvc.DeployAgent(server); err != nil {
		server.Status = "offline"
		h.store.UpdateServer(server)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	server.Status = "online"
	h.store.UpdateServer(server)
	c.JSON(http.StatusOK, gin.H{"ok": true, "status": "online"})
}

func (h *ServerHandler) TestSSH(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil { c.JSON(http.StatusNotFound, gin.H{"error": "server not found"}); return }

	msg, err := h.deploySvc.TestSSH(server)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": msg})
}
