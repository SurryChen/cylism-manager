package cluster

import (
	"errors"
	"net/http"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/accesscert"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AccessCertificateStore interface {
	accesscert.Repository
	GetCluster(uint) (*model.Cluster, error)
	GetAccessCertificate(uint) (*model.AccessCertificate, error)
	RevokeAccessCertificate(uint, time.Time) error
	DeleteAccessCertificate(uint) error
}

type AccessCertificateHandler struct {
	store   AccessCertificateStore
	service *accesscert.Service
}

func NewAccessCertificateHandler(st AccessCertificateStore, key []byte) *AccessCertificateHandler {
	return &AccessCertificateHandler{store: st, service: accesscert.NewService(st, key)}
}

func (h *AccessCertificateHandler) Service() *accesscert.Service { return h.service }

func (h *AccessCertificateHandler) CA(c *gin.Context) {
	ca, err := h.store.GetAccessCA()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.Success(c, nil)
		return
	}
	if err != nil {
		apiShared.DBError(c, "读取访问 CA 失败")
		return
	}
	apiShared.Success(c, ca)
}

func (h *AccessCertificateHandler) InitializeCA(c *gin.Context) {
	ca, err := h.service.InitializeCA()
	if errors.Is(err, store.ErrAccessCAExists) {
		apiShared.Conflict(c, "访问 CA 已创建")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "创建访问 CA 失败")
		return
	}
	apiShared.Success(c, ca)
}

func (h *AccessCertificateHandler) List(c *gin.Context) {
	items, err := h.store.ListAccessCertificates()
	if err != nil {
		apiShared.DBError(c, "读取访问证书失败")
		return
	}
	apiShared.Success(c, items)
}

func (h *AccessCertificateHandler) Issue(c *gin.Context) {
	var request struct {
		ClusterID uint   `json:"cluster_id"`
		Kind      string `json:"kind"`
		CSR       string `json:"csr"`
	}
	if c.ShouldBindJSON(&request) != nil || request.ClusterID == 0 {
		apiShared.BadRequest(c, "请选择集群和证书类型")
		return
	}
	if request.Kind != accesscert.AgentServer && request.Kind != accesscert.PlatformClient {
		apiShared.ValidationError(c, "证书类型无效")
		return
	}
	if request.Kind == accesscert.AgentServer && strings.TrimSpace(request.CSR) == "" {
		apiShared.ValidationError(c, "请填写 Agent 生成的 CSR")
		return
	}
	if _, err := h.store.GetCluster(request.ClusterID); errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	} else if err != nil {
		apiShared.DBError(c, "读取集群失败")
		return
	}
	if _, err := h.store.GetAccessCA(); errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.Conflict(c, "请先创建访问 CA")
		return
	} else if err != nil {
		apiShared.DBError(c, "读取访问 CA 失败")
		return
	}
	item, err := h.service.Issue(request.ClusterID, request.Kind, request.CSR)
	if err != nil {
		if strings.Contains(err.Error(), "CSR") || strings.Contains(err.Error(), "证书类型") || strings.Contains(err.Error(), "不需要 CSR") {
			apiShared.ValidationError(c, err.Error())
			return
		}
		apiShared.InternalError(c, "签发访问证书失败")
		return
	}
	apiShared.Success(c, item)
}

func (h *AccessCertificateHandler) Certificate(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "证书 ID 无效")
		return
	}
	item, err := h.store.GetAccessCertificate(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "访问证书不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, "读取访问证书失败")
		return
	}
	apiShared.Success(c, gin.H{"certificate_pem": item.CertificatePEM})
}

func (h *AccessCertificateHandler) Revoke(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "证书 ID 无效")
		return
	}
	item, err := h.store.GetAccessCertificate(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "访问证书不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, "读取访问证书失败")
		return
	}
	if item.RevokedAt != nil {
		apiShared.Conflict(c, "证书已吊销")
		return
	}
	if item.InUse {
		apiShared.Conflict(c, "证书正在使用，请先切换到其他证书")
		return
	}
	if err := h.store.RevokeAccessCertificate(id, time.Now().UTC()); err != nil {
		apiShared.DBError(c, "吊销访问证书失败")
		return
	}
	apiShared.SuccessWithMessage(c, nil, "访问证书已吊销")
}

func (h *AccessCertificateHandler) Delete(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "证书 ID 无效")
		return
	}
	item, err := h.store.GetAccessCertificate(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "访问证书不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, "读取访问证书失败")
		return
	}
	if item.InUse {
		apiShared.Conflict(c, "证书正在使用，不能删除")
		return
	}
	if err := h.store.DeleteAccessCertificate(id); errors.Is(err, store.ErrAccessCertificateInUse) {
		apiShared.Conflict(c, "证书正在使用，不能删除")
		return
	} else if err != nil {
		apiShared.DBError(c, "删除访问证书失败")
		return
	}
	apiShared.SuccessWithMessage(c, nil, "访问证书已删除")
}

func (h *AccessCertificateHandler) CRL(c *gin.Context) {
	crl, err := h.service.CRL()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.Error(c, http.StatusNotFound, apiShared.CodeNotFound, "请先创建访问 CA")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "生成证书吊销列表失败")
		return
	}
	apiShared.Success(c, gin.H{"crl_pem": crl})
}
