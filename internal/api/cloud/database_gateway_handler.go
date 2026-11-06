package cloud

import (
	"errors"
	"fmt"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/database/gateway"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *DatabaseHandler) WithPrivateGateways(service *gateway.Service) *DatabaseHandler {
	h.gateways = service
	return h
}

func (h *DatabaseHandler) GetPrivateGateway(c *gin.Context) {
	instance, ok := h.get(c)
	if !ok {
		return
	}
	var config model.DatabaseGateway
	err := h.store.DB().Where("instance_id = ?", instance.ID).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.Success(c, gin.H{"gateway": nil, "tasks": []model.DatabaseGatewayTask{}})
		return
	}
	if err != nil {
		apiShared.DBError(c, "读取私网网关失败")
		return
	}
	var tasks []model.DatabaseGatewayTask
	if err := h.store.DB().Where("gateway_id = ?", config.ID).Order("id desc").Limit(20).Find(&tasks).Error; err != nil {
		apiShared.DBError(c, "读取网关任务失败")
		return
	}
	if tasks == nil {
		tasks = []model.DatabaseGatewayTask{}
	}
	apiShared.Success(c, gin.H{"gateway": config, "tasks": tasks, "service_host": gateway.ServiceHost(config.ID), "service_port": 3306, "tls_server_name": instance.Host, "validation_scope": "各可用节点的 DNS、TCP 与 MySQL 握手；不验证账号认证或 TLS"})
}

func (h *DatabaseHandler) ConfigurePrivateGateway(c *gin.Context) {
	instance, ok := h.get(c)
	if !ok {
		return
	}
	if h.gateways == nil {
		apiShared.ValidationError(c, "私网网关后台服务未配置")
		return
	}
	var input struct {
		ServerID       uint     `json:"server_id"`
		ClusterID      uint     `json:"cluster_id"`
		ListenIP       string   `json:"listen_ip"`
		ListenPort     int      `json:"listen_port"`
		AllowedSources []string `json:"allowed_sources"`
		ProbeImage     string   `json:"probe_image"`
	}
	if c.ShouldBindJSON(&input) != nil {
		apiShared.BadRequest(c, "网关配置无效")
		return
	}
	config, err := h.gateways.Configure(instance, model.DatabaseGateway{ServerID: input.ServerID, ClusterID: input.ClusterID, ListenIP: input.ListenIP, ListenPort: input.ListenPort, AllowedSources: input.AllowedSources, ProbeImage: input.ProbeImage})
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, config)
}

func (h *DatabaseHandler) PrivateGatewayAction(c *gin.Context) {
	instance, ok := h.get(c)
	if !ok {
		return
	}
	if h.gateways == nil {
		apiShared.ValidationError(c, "私网网关后台服务未配置")
		return
	}
	var input struct {
		Action string `json:"action"`
	}
	if c.ShouldBindJSON(&input) != nil {
		apiShared.BadRequest(c, "网关操作无效")
		return
	}
	task, err := h.gateways.Queue(instance.ID, input.Action)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, task)
}

func (h *DatabaseHandler) PrivateGatewayTaskLogs(c *gin.Context) {
	instance, ok := h.get(c)
	if !ok {
		return
	}
	taskID, err := apiShared.ParsePositiveID(c.Param("taskID"))
	if err != nil {
		apiShared.BadRequest(c, "任务 ID 无效")
		return
	}
	var config model.DatabaseGateway
	if err := h.store.DB().Where("instance_id = ?", instance.ID).First(&config).Error; err != nil {
		apiShared.NotFound(c, "网关不存在")
		return
	}
	var task model.DatabaseGatewayTask
	if err := h.store.DB().Where("id = ? AND gateway_id = ?", taskID, config.ID).First(&task).Error; err != nil {
		apiShared.NotFound(c, "任务不存在")
		return
	}
	var logs []model.DatabaseGatewayTaskLog
	if err := h.store.DB().Where("task_id = ?", task.ID).Order("id asc").Find(&logs).Error; err != nil {
		apiShared.DBError(c, "读取任务日志失败")
		return
	}
	if logs == nil {
		logs = []model.DatabaseGatewayTaskLog{}
	}
	apiShared.Success(c, logs)
}

func (h *DatabaseHandler) privateGatewayBinding(tx *gorm.DB, item *model.DatabaseInstance, clusterID uint) (string, int, error) {
	var config model.DatabaseGateway
	err := tx.Where("instance_id = ?", item.ID).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if item.ConnectionMode == "gateway" {
			return "", 0, fmt.Errorf("请先配置私网网关并完成连通性验证")
		}
		return item.Host, item.Port, nil
	}
	if err != nil {
		return "", 0, err
	}
	if clusterID != config.ClusterID {
		return "", 0, fmt.Errorf("应用必须位于私网网关接入的集群")
	}
	if config.Status != "ready" {
		return "", 0, fmt.Errorf("请先完成私网网关部署与连通性验证")
	}
	var active int64
	if err := tx.Model(&model.DatabaseGatewayTask{}).Where("gateway_id = ? AND status IN ?", config.ID, []string{"queued", "running"}).Count(&active).Error; err != nil {
		return "", 0, err
	}
	if active > 0 {
		return "", 0, fmt.Errorf("请等待私网网关任务完成后再绑定")
	}
	return gateway.ServiceHost(config.ID), 3306, nil
}
