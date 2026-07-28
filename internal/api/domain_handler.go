package api

import (
	"net/http"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type DomainHandler struct{ store *store.Store }
type domainRequest struct {
	Hostname    string `json:"hostname"`
	IssuerRef   string `json:"issuer_ref"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

func NewDomainHandler(s *store.Store) *DomainHandler { return &DomainHandler{store: s} }
func (h *DomainHandler) List(c *gin.Context) {
	domains, err := h.store.ListManagedDomains()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, domains)
}
func (h *DomainHandler) Create(c *gin.Context) {
	var req domainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名定义无效")
		return
	}
	domain, err := domainFromRequest(req, nil)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.store.CreateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已存在")
		return
	}
	model.Success(c, domain)
}
func (h *DomainHandler) Update(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名 ID 无效")
		return
	}
	current, err := h.store.GetManagedDomain(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "域名不存在")
		return
	}
	var req domainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名定义无效")
		return
	}
	domain, err := domainFromRequest(req, current)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.store.UpdateManagedDomain(domain); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "域名已存在")
		return
	}
	model.Success(c, domain)
}
func (h *DomainHandler) Delete(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "域名 ID 无效")
		return
	}
	if err := h.store.DeleteManagedDomain(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": id})
}
func domainFromRequest(req domainRequest, current *model.ManagedDomain) (*model.ManagedDomain, error) {
	hostname := strings.ToLower(strings.TrimSpace(req.Hostname))
	if hostname == "" || strings.ContainsAny(hostname, " /:@") {
		return nil, errInvalid("域名格式无效")
	}
	domain := &model.ManagedDomain{Hostname: hostname, IssuerRef: strings.TrimSpace(req.IssuerRef), Description: strings.TrimSpace(req.Description), Enabled: req.Enabled}
	if current != nil {
		domain.ID = current.ID
		domain.CreatedAt = current.CreatedAt
	}
	return domain, nil
}
