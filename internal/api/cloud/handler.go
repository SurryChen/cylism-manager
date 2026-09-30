package cloud

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	service *cloudservice.Service
	audit   auditservice.Repository
}

func NewHandler(service *cloudservice.Service, audit auditservice.Repository) *Handler {
	return &Handler{service: service, audit: audit}
}

func (h *Handler) ListProviders(c *gin.Context) {
	apiShared.Success(c, h.service.ProviderCatalog())
}

type connectionRequest struct {
	Name          string  `json:"name"`
	Provider      string  `json:"provider"`
	Credentials   *string `json:"credentials"`
	Configuration string  `json:"configuration"`
}
type confirmRequest struct {
	Confirm bool `json:"confirm"`
}

func (h *Handler) ListConnections(c *gin.Context) {
	items, err := h.service.ListConnections()
	if err != nil {
		dbError(c, err)
		return
	}
	apiShared.Success(c, items)
}
func (h *Handler) CreateConnection(c *gin.Context) {
	var request connectionRequest
	if c.ShouldBindJSON(&request) != nil {
		apiShared.BadRequest(c, "云连接定义无效")
		return
	}
	item := connectionFromRequest(request, nil)
	item.CreatedBy = apiShared.UserID(c)
	if err := h.service.SaveConnection(item, request.Credentials); err != nil {
		validation(c, err)
		return
	}
	item.CredentialConfigured = true
	h.record(c, "cloud.connection.create", item.ID, item.Name, model.AuditOutcomeSucceeded, "创建云连接")
	apiShared.Success(c, item)
}
func (h *Handler) UpdateConnection(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	current, err := h.service.GetConnection(id)
	if err != nil {
		notFound(c, err, "云连接不存在")
		return
	}
	var request connectionRequest
	if c.ShouldBindJSON(&request) != nil {
		apiShared.BadRequest(c, "云连接定义无效")
		return
	}
	item := connectionFromRequest(request, current)
	if err := h.service.SaveConnection(item, request.Credentials); err != nil {
		validation(c, err)
		return
	}
	item.CredentialConfigured = item.EncryptedCredentials != ""
	h.record(c, "cloud.connection.update", item.ID, item.Name, model.AuditOutcomeSucceeded, "更新云连接")
	apiShared.Success(c, item)
}
func (h *Handler) DeleteConnection(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteConnection(id); err != nil {
		dbError(c, err)
		return
	}
	h.record(c, "cloud.connection.delete", id, "云连接", model.AuditOutcomeSucceeded, "删除云连接")
	apiShared.Success(c, gin.H{"id": id})
}
func (h *Handler) InspectPermissions(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	result, err := h.service.InspectPermissions(c.Request.Context(), id)
	if err != nil {
		providerError(c, err)
		h.record(c, "cloud.connection.permissions.inspect", id, "云连接", model.AuditOutcomeFailed, "查看云连接权限失败")
		return
	}
	h.record(c, "cloud.connection.permissions.inspect", id, "云连接", model.AuditOutcomeSucceeded, "查看云连接权限")
	apiShared.Success(c, result)
}

func (h *Handler) ListZones(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	items, err := h.service.ListZones(c.Request.Context(), id)
	if err != nil {
		providerError(c, err)
		return
	}
	apiShared.Success(c, items)
}
func (h *Handler) ListRecords(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	items, err := h.service.ListRecords(c.Request.Context(), id, c.Query("zone"), page, size)
	if err != nil {
		providerError(c, err)
		return
	}
	apiShared.Success(c, items)
}
func (h *Handler) CreateRecord(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	var input struct {
		Zone string `json:"zone"`
		cloudservice.DNSRecordInput
	}
	if c.ShouldBindJSON(&input) != nil {
		apiShared.BadRequest(c, "DNS 记录无效")
		return
	}
	item, err := h.service.CreateRecord(c.Request.Context(), id, input.Zone, input.DNSRecordInput)
	if err != nil {
		validation(c, err)
		return
	}
	h.record(c, "cloud.dns.record.create", id, input.Zone+"/"+item.RR, model.AuditOutcomeSucceeded, "创建 DNS 记录")
	apiShared.Success(c, item)
}
func (h *Handler) UpdateRecord(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	var input cloudservice.DNSRecordInput
	if c.ShouldBindJSON(&input) != nil {
		apiShared.BadRequest(c, "DNS 记录无效")
		return
	}
	item, err := h.service.UpdateRecord(c.Request.Context(), id, c.Param("recordID"), input)
	if err != nil {
		validation(c, err)
		return
	}
	h.record(c, "cloud.dns.record.update", id, item.ID, model.AuditOutcomeSucceeded, "更新 DNS 记录")
	apiShared.Success(c, item)
}
func (h *Handler) DeleteRecord(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	var request confirmRequest
	_ = c.ShouldBindJSON(&request)
	if err := h.service.DeleteRecord(c.Request.Context(), id, c.Param("recordID"), request.Confirm); err != nil {
		validation(c, err)
		h.record(c, "cloud.dns.record.delete", id, c.Param("recordID"), model.AuditOutcomeFailed, "删除 DNS 记录失败")
		return
	}
	h.record(c, "cloud.dns.record.delete", id, c.Param("recordID"), model.AuditOutcomeSucceeded, "删除 DNS 记录")
	apiShared.Success(c, gin.H{"id": c.Param("recordID")})
}

func (h *Handler) ListContainers(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	items, err := h.service.ListContainers(c.Request.Context(), id)
	if err != nil {
		providerError(c, err)
		return
	}
	apiShared.Success(c, items)
}
func (h *Handler) CreateContainer(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	var input cloudservice.ContainerInput
	if c.ShouldBindJSON(&input) != nil {
		apiShared.BadRequest(c, "存储容器定义无效")
		return
	}
	item, err := h.service.CreateContainer(c.Request.Context(), id, input)
	if err != nil {
		validation(c, err)
		return
	}
	h.record(c, "cloud.object_storage.container.create", id, item.Name, model.AuditOutcomeSucceeded, "创建存储容器")
	apiShared.Success(c, item)
}
func (h *Handler) UpdateContainer(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	var input cloudservice.ContainerInput
	if c.ShouldBindJSON(&input) != nil {
		apiShared.BadRequest(c, "存储容器配置无效")
		return
	}
	item, err := h.service.UpdateContainer(c.Request.Context(), id, c.Param("container"), input)
	if err != nil {
		validation(c, err)
		return
	}
	h.record(c, "cloud.object_storage.container.update", id, item.Name, model.AuditOutcomeSucceeded, "更新存储容器")
	apiShared.Success(c, item)
}
func (h *Handler) DeleteContainer(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	var request confirmRequest
	_ = c.ShouldBindJSON(&request)
	if err := h.service.DeleteContainer(c.Request.Context(), id, c.Param("container"), request.Confirm, c.Query("region")); err != nil {
		validation(c, err)
		h.record(c, "cloud.object_storage.container.delete", id, c.Param("container"), model.AuditOutcomeFailed, "删除存储容器失败")
		return
	}
	h.record(c, "cloud.object_storage.container.delete", id, c.Param("container"), model.AuditOutcomeSucceeded, "删除存储容器")
	apiShared.Success(c, gin.H{"name": c.Param("container")})
}

func (h *Handler) ListObjects(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	items, err := h.service.ListObjects(c.Request.Context(), id, c.Param("container"), c.Query("prefix"), c.Query("token"), c.Query("region"))
	if err != nil {
		providerError(c, err)
		return
	}
	apiShared.Success(c, items)
}
func (h *Handler) UploadObject(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		apiShared.BadRequest(c, "请选择要上传的文件")
		return
	}
	defer file.Close()
	key := strings.TrimSpace(c.PostForm("key"))
	if key == "" {
		key = header.Filename
	}
	item, err := h.service.UploadObject(c.Request.Context(), id, c.Param("container"), key, io.LimitReader(file, 1024*1024*1024+1), header.Size, header.Header.Get("Content-Type"), c.Query("region"))
	if err != nil {
		validation(c, err)
		return
	}
	h.record(c, "cloud.object_storage.object.upload", id, c.Param("container")+"/"+key, model.AuditOutcomeSucceeded, "上传对象")
	apiShared.Success(c, item)
}
func (h *Handler) DownloadObject(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	key := c.Query("key")
	item, err := h.service.DownloadObject(c.Request.Context(), id, c.Param("container"), key, c.Query("region"))
	if err != nil {
		providerError(c, err)
		return
	}
	defer item.Body.Close()
	c.Header("Content-Disposition", "attachment; filename=\""+path.Base(item.Filename)+"\"")
	c.DataFromReader(http.StatusOK, item.Size, item.ContentType, item.Body, nil)
}
func (h *Handler) DeleteObject(c *gin.Context) {
	id, ok := connectionID(c)
	if !ok {
		return
	}
	var request struct {
		Key     string `json:"key"`
		Confirm bool   `json:"confirm"`
	}
	if c.ShouldBindJSON(&request) != nil {
		apiShared.BadRequest(c, "对象删除请求无效")
		return
	}
	if err := h.service.DeleteObject(c.Request.Context(), id, c.Param("container"), request.Key, request.Confirm, c.Query("region")); err != nil {
		validation(c, err)
		h.record(c, "cloud.object_storage.object.delete", id, c.Param("container")+"/"+request.Key, model.AuditOutcomeFailed, "删除对象失败")
		return
	}
	h.record(c, "cloud.object_storage.object.delete", id, c.Param("container")+"/"+request.Key, model.AuditOutcomeSucceeded, "删除对象")
	apiShared.Success(c, gin.H{"key": request.Key})
}

func connectionFromRequest(request connectionRequest, current *model.CloudConnection) *model.CloudConnection {
	item := &model.CloudConnection{Name: strings.TrimSpace(request.Name), Provider: strings.TrimSpace(request.Provider), Configuration: strings.TrimSpace(request.Configuration)}
	if current != nil {
		item.ID, item.CreatedAt, item.CreatedBy, item.EncryptedCredentials, item.DNSStatus, item.ObjectStorageStatus, item.LastValidationAt, item.LastValidationError = current.ID, current.CreatedAt, current.CreatedBy, current.EncryptedCredentials, current.DNSStatus, current.ObjectStorageStatus, current.LastValidationAt, current.LastValidationError
	}
	return item
}
func (h *Handler) record(c *gin.Context, action string, id uint, target string, outcome string, summary string) {
	if h.audit == nil {
		return
	}
	_ = auditservice.NewService(h.audit).Record(auditservice.AuditEventInput{Action: action, ResourceType: "cloud_connection", ResourceID: id, TargetName: target, Actor: apiShared.ActorFromContext(c), Source: model.AuditSourceAPI, Outcome: outcome, Summary: summary, RequestID: apiShared.RequestID(c)})
}
func connectionID(c *gin.Context) (uint, bool) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "云连接 ID 无效")
		return 0, false
	}
	return id, true
}
func validation(c *gin.Context, err error) { apiShared.ValidationError(c, err.Error()) }
func providerError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "云连接不存在")
		return
	}
	apiShared.Error(c, http.StatusBadRequest, apiShared.CodeValidationFail, err.Error())
}
func notFound(c *gin.Context, err error, message string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, message)
		return
	}
	dbError(c, err)
}
func dbError(c *gin.Context, _ error) { apiShared.DBError(c, "读取云连接失败") }

var _ repository.CloudConnectionRepository
