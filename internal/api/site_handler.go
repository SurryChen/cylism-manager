package api

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"

	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
)

type SiteHandler struct {
	store repository.SiteRepository
}

func NewSiteHandler(s repository.SiteRepository) *SiteHandler {
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
		apiShared.BadRequest(c, err.Error())
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
		apiShared.Conflict(c, err.Error())
		return
	}
	model.Success(c, site)
}

func (h *SiteHandler) List(c *gin.Context) {
	serverIDStr := c.Query("server_id")
	if serverIDStr != "" {
		id, err := apiShared.ParsePositiveID(serverIDStr)
		if err != nil {
			apiShared.BadRequest(c, "invalid server_id")
			return
		}
		sites, err := h.store.ListSitesByServer(uint(id))
		if err != nil {
			apiShared.InternalError(c, err.Error())
			return
		}
		model.Success(c, sites)
		return
	}
	sites, err := h.store.ListSites()
	if err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.Success(c, sites)
}

func (h *SiteHandler) Get(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "site ID 无效")
		return
	}
	site, err := h.store.GetSite(id)
	if err != nil {
		apiShared.NotFound(c, "site not found")
		return
	}
	model.Success(c, site)
}

func (h *SiteHandler) Update(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "site ID 无效")
		return
	}
	site, err := h.store.GetSite(id)
	if err != nil {
		apiShared.NotFound(c, "site not found")
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		apiShared.BadRequest(c, err.Error())
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
		apiShared.InternalError(c, err.Error())
		return
	}
	model.Success(c, site)
}

func (h *SiteHandler) Delete(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "site ID 无效")
		return
	}
	if err := h.store.DeleteSite(id); err != nil {
		apiShared.InternalError(c, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "操作成功")
}

// -- Certificate stubs --

func (h *SiteHandler) IssueCert(c *gin.Context) {
	// TODO: acme.sh 集成
	model.SuccessWithMessage(c, nil, "issue cert - not implemented yet")
}

func (h *SiteHandler) RenewCert(c *gin.Context) {
	model.SuccessWithMessage(c, nil, "renew cert - not implemented yet")
}

func (h *SiteHandler) RevokeCert(c *gin.Context) {
	model.SuccessWithMessage(c, nil, "revoke cert - not implemented yet")
}

// -- NGINX stubs --

func (h *SiteHandler) GenerateNginx(c *gin.Context) {
	model.SuccessWithMessage(c, nil, "generate nginx - K3s implementation pending")
}

func (h *SiteHandler) ReloadNginx(c *gin.Context) {
	model.SuccessWithMessage(c, nil, "reload nginx - K3s implementation pending")
}
