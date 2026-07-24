package api

import (
	"net/http"
	"strconv"

	"github.com/cylism/cylism-manager/internal/model"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type SiteHandler struct {
	store *store.Store
}

func NewSiteHandler(s *store.Store) *SiteHandler {
	return &SiteHandler{store: s}
}

type createSiteReq struct {
	ServerID   uint   `json:"server_id" binding:"required"`
	Domain     string `json:"domain" binding:"required"`
	Port       int    `json:"port"`
	SSLEnabled bool   `json:"ssl_enabled"`
	RootPath   string `json:"root_path"`
	Upstream   string `json:"upstream"`
	Locations  string `json:"locations"`
}

func (h *SiteHandler) Create(c *gin.Context) {
	var req createSiteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 80
	}

	site := &model.Site{
		ServerID:   req.ServerID,
		Domain:     req.Domain,
		Port:       req.Port,
		SSLEnabled: req.SSLEnabled,
		RootPath:   req.RootPath,
		Managed:    true,
		Upstream:   req.Upstream,
		Locations:  req.Locations,
	}
	if err := h.store.CreateSite(site); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, site)
}

func (h *SiteHandler) List(c *gin.Context) {
	serverIDStr := c.Query("server_id")
	if serverIDStr != "" {
		id, err := strconv.ParseUint(serverIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server_id"})
			return
		}
		sites, err := h.store.ListSitesByServer(uint(id))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, sites)
		return
	}
	sites, err := h.store.ListSites()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sites)
}

func (h *SiteHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	site, err := h.store.GetSite(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	c.JSON(http.StatusOK, site)
}

func (h *SiteHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	site, err := h.store.GetSite(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if v, ok := updates["root_path"]; ok {
		site.RootPath = v.(string)
	}
	if v, ok := updates["ssl_enabled"]; ok {
		site.SSLEnabled = v.(bool)
	}
	if v, ok := updates["port"]; ok {
		site.Port = int(v.(float64))
	}
	if v, ok := updates["managed"]; ok {
		site.Managed = v.(bool)
	}
	if v, ok := updates["upstream"]; ok {
		site.Upstream = v.(string)
	}
	if v, ok := updates["locations"]; ok {
		site.Locations = v.(string)
	}
	if err := h.store.UpdateSite(site); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, site)
}

func (h *SiteHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.store.DeleteSite(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// -- Certificate stubs --

func (h *SiteHandler) IssueCert(c *gin.Context) {
	// TODO: acme.sh 集成
	c.JSON(http.StatusOK, gin.H{"message": "issue cert - not implemented yet"})
}

func (h *SiteHandler) RenewCert(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "renew cert - not implemented yet"})
}

func (h *SiteHandler) RevokeCert(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "revoke cert - not implemented yet"})
}

// -- NGINX stubs --

func (h *SiteHandler) GenerateNginx(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "generate nginx - K3s implementation pending"})
}

func (h *SiteHandler) ReloadNginx(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "reload nginx - K3s implementation pending"})
}
