// Package gateway manages private, bounded TCP forwarding without public RDS access.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/cylism/cylism-manager/internal/transport"
	"gorm.io/gorm"
)

type ClientFactory func(context.Context, uint) (*k8s.Client, error)
type Executor func(context.Context, time.Duration, *model.Server, string) ([]byte, error)

type Service struct {
	store  *store.Store
	client ClientFactory
	exec   Executor
	mu     sync.Mutex
	// Kept injectable so recovery and task failure behavior can be tested without SSH.
	apply  func(context.Context, *k8s.Client, *model.DatabaseGateway) error
	remove func(context.Context, *k8s.Client, *model.DatabaseGateway) error
	probe  func(context.Context, *k8s.Client, *model.DatabaseGateway, func(string) error) error
}

func New(s *store.Store, key []byte, factory ClientFactory) *Service {
	return &Service{store: s, client: factory, exec: func(ctx context.Context, timeout time.Duration, server *model.Server, command string) ([]byte, error) {
		return transport.SSHExecServerContext(ctx, timeout, server, key, command)
	}, apply: ApplyService, remove: DeleteService, probe: Probe}
}

// Configure queues deployment atomically with its configuration. An in-flight
// operation keeps an immutable configuration, even across a process restart.
func (s *Service) Configure(instance *model.DatabaseInstance, input model.DatabaseGateway) (*model.DatabaseGateway, error) {
	if err := Validate(instance, &input); err != nil {
		return nil, err
	}
	if _, err := s.store.GetServer(input.ServerID); err != nil {
		return nil, fmt.Errorf("网关服务器不存在")
	}
	cluster, err := s.store.GetCluster(input.ClusterID)
	if err != nil || cluster.Status != "ready" {
		return nil, fmt.Errorf("请选择已就绪的应用集群")
	}
	var result model.DatabaseGateway
	err = s.store.DB().Transaction(func(tx *gorm.DB) error {
		var current model.DatabaseInstance
		if err := tx.First(&current, instance.ID).Error; err != nil {
			return fmt.Errorf("数据库实例已被移除")
		}
		if current.Mode != "external" || current.Host != instance.Host || current.Port != instance.Port {
			return fmt.Errorf("数据库实例配置已变化，请刷新后重试")
		}
		err := tx.Where("instance_id = ?", instance.ID).First(&result).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if result.ID != 0 {
			if err := idle(tx, result.ID); err != nil {
				return err
			}
			if result.ServerID != input.ServerID || result.ClusterID != input.ClusterID || result.ListenIP != input.ListenIP || result.ListenPort != input.ListenPort {
				return fmt.Errorf("现有网关的服务器、集群和监听地址不可变更；请停用并移除实例记录后重新接入")
			}
		} else {
			var bindings int64
			if err := tx.Model(&model.DatabaseBinding{}).Where("instance_id = ?", instance.ID).Count(&bindings).Error; err != nil {
				return err
			}
			if bindings > 0 {
				return fmt.Errorf("实例已有应用绑定，请先解除绑定再改用私网网关")
			}
			result.InstanceID = instance.ID
		}
		var collision int64
		if err := tx.Model(&model.DatabaseGateway{}).Where("server_id = ? AND listen_ip = ? AND listen_port = ? AND id <> ?", input.ServerID, input.ListenIP, input.ListenPort, result.ID).Count(&collision).Error; err != nil {
			return err
		}
		if collision > 0 {
			return fmt.Errorf("该网关监听地址和端口已被其他数据库使用")
		}
		result.ServerID, result.ClusterID, result.ListenIP, result.ListenPort = input.ServerID, input.ClusterID, input.ListenIP, input.ListenPort
		result.AllowedSources, result.ProbeImage, result.Status, result.LastError = input.AllowedSources, input.ProbeImage, "pending", ""
		if err := tx.Model(&current).Update("connection_mode", "gateway").Error; err != nil {
			return err
		}
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		return tx.Create(&model.DatabaseGatewayTask{GatewayID: result.ID, Kind: "deploy", Status: "queued", Progress: "等待后台部署私网网关"}).Error
	})
	return &result, err
}

func idle(tx *gorm.DB, id uint) error {
	var count int64
	if err := tx.Model(&model.DatabaseGatewayTask{}).Where("gateway_id = ? AND status IN ?", id, []string{"queued", "running"}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("网关已有进行中的任务，请等待完成")
	}
	return nil
}

func (s *Service) Queue(instanceID uint, kind string) (*model.DatabaseGatewayTask, error) {
	if kind != "deploy" && kind != "check" && kind != "disable" {
		return nil, fmt.Errorf("不支持的网关操作")
	}
	var task model.DatabaseGatewayTask
	err := s.store.DB().Transaction(func(tx *gorm.DB) error {
		var instance model.DatabaseInstance
		if err := tx.First(&instance, instanceID).Error; err != nil {
			return fmt.Errorf("数据库实例已被移除")
		}
		var gateway model.DatabaseGateway
		if err := tx.Where("instance_id = ?", instanceID).First(&gateway).Error; err != nil {
			return fmt.Errorf("私网网关尚未配置")
		}
		if err := idle(tx, gateway.ID); err != nil {
			return err
		}
		if kind == "check" && gateway.Status == "disabled" {
			return fmt.Errorf("请先启用网关")
		}
		task = model.DatabaseGatewayTask{GatewayID: gateway.ID, Kind: kind, Status: "queued", Progress: "等待后台执行"}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		status := "checking"
		if kind == "deploy" {
			status = "pending"
		}
		if kind == "disable" {
			status = "disabling"
		}
		return tx.Model(&gateway).Updates(map[string]any{"status": status, "last_error": ""}).Error
	})
	return &task, err
}

func (s *Service) append(task *model.DatabaseGatewayTask, text string) error {
	return s.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.DatabaseGatewayTaskLog{TaskID: task.ID, Message: text}).Error; err != nil {
			return err
		}
		return tx.Model(task).Update("progress", text).Error
	})
}

// Reconcile is lifecycle-owned by bootstrap. Queued and interrupted running
// tasks are idempotently replayed; failures require an explicit retry.
func (s *Service) Reconcile(ctx context.Context) {
	if !s.mu.TryLock() {
		return
	}
	defer s.mu.Unlock()
	var tasks []model.DatabaseGatewayTask
	if s.store.DB().Where("status IN ?", []string{"queued", "running"}).Order("id").Limit(20).Find(&tasks).Error != nil {
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		s.run(ctx, &task)
	}
	if ctx.Err() != nil {
		return
	}
	var gateways []model.DatabaseGateway
	if s.store.DB().Where("status IN ? AND (last_checked_at IS NULL OR last_checked_at < ?)", []string{"ready", "degraded"}, time.Now().Add(-time.Minute)).Find(&gateways).Error != nil {
		return
	}
	for _, gateway := range gateways {
		if ctx.Err() != nil {
			return
		}
		server, err := s.store.GetServer(gateway.ServerID)
		if err == nil {
			_, err = s.exec(ctx, 45*time.Second, server, Privileged(server, HealthCommand(&gateway)))
		}
		if ctx.Err() != nil {
			return
		}
		status, detail := "ready", ""
		if err != nil {
			status, detail = "degraded", "网关服务检查失败；请检查服务器或重新部署网关"
		}
		// SSH runs outside a transaction. A newly queued operation must keep its
		// state even when this older health check finishes afterwards.
		active := s.store.DB().Model(&model.DatabaseGatewayTask{}).Select("1").
			Where("gateway_id = ? AND status IN ?", gateway.ID, []string{"queued", "running"})
		_ = s.store.DB().Model(&model.DatabaseGateway{}).
			Where("id = ? AND status = ? AND updated_at = ?", gateway.ID, gateway.Status, gateway.UpdatedAt).
			Where("NOT EXISTS (?)", active).
			Updates(map[string]any{"status": status, "last_error": detail, "last_checked_at": time.Now()}).Error
	}
}

func (s *Service) run(ctx context.Context, task *model.DatabaseGatewayTask) {
	log := func(message string) error { return s.append(task, message) }
	resuming := task.Status == "running"
	if err := s.store.DB().Model(task).Update("status", "running").Error; err != nil {
		return
	}
	message := "开始私网网关任务"
	if resuming {
		message = "平台恢复后重新检查并继续任务（操作可重复执行）"
	}
	if err := log(message); err != nil {
		return
	}
	var gateway model.DatabaseGateway
	if err := s.store.DB().First(&gateway, task.GatewayID).Error; err != nil {
		s.finish(task, nil, err)
		return
	}
	err := s.execute(ctx, task, &gateway, log)
	// A shutdown is not a terminal failure; the next process replays this task.
	if ctx.Err() != nil {
		return
	}
	s.finish(task, &gateway, err)
}

func (s *Service) finish(task *model.DatabaseGatewayTask, gateway *model.DatabaseGateway, failure error) {
	status, state, message := "succeeded", "ready", "各可用节点 3/3 次网络验证通过；账号认证和 TLS 仍需应用配置验证"
	if task.Kind == "disable" {
		state, message = "disabled", "已停用网关并移除集群 Service"
	}
	detail := ""
	if failure != nil {
		status, state, message, detail = "failed", "error", "任务失败："+failure.Error(), failure.Error()
	}
	_ = s.store.DB().Transaction(func(tx *gorm.DB) error {
		if gateway != nil {
			changes := map[string]any{"status": state, "last_error": detail, "last_checked_at": time.Now()}
			if failure == nil && task.Kind != "disable" {
				changes["last_verified_at"] = time.Now()
			}
			if err := tx.Model(gateway).Updates(changes).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&model.DatabaseGatewayTaskLog{TaskID: task.ID, Message: message}).Error; err != nil {
			return err
		}
		return tx.Model(task).Updates(map[string]any{"status": status, "progress": message}).Error
	})
}

func (s *Service) execute(ctx context.Context, task *model.DatabaseGatewayTask, config *model.DatabaseGateway, log func(string) error) error {
	if s.client == nil || s.exec == nil {
		return fmt.Errorf("网关执行器未配置")
	}
	server, err := s.store.GetServer(config.ServerID)
	if err != nil {
		return fmt.Errorf("读取网关服务器失败")
	}
	if task.Kind == "disable" {
		if err := log("正在停止网关转发并取消开机启动"); err != nil {
			return err
		}
		if output, err := s.exec(ctx, time.Minute, server, Privileged(server, DisableCommand(config.ID))); err != nil {
			return commandError("停用转发服务失败", output, err)
		}
		client, err := s.client(ctx, config.ClusterID)
		if err != nil {
			return err
		}
		return s.remove(ctx, client, config)
	}
	var instance model.DatabaseInstance
	if err := s.store.DB().First(&instance, config.InstanceID).Error; err != nil {
		return err
	}
	if err := Validate(&instance, config); err != nil {
		return err
	}
	if task.Kind == "deploy" {
		if err := log("检查网关 Tailscale、目标私网 DNS 和 MySQL 握手，然后安装受管 systemd 转发服务"); err != nil {
			return err
		}
		if output, err := s.exec(ctx, 2*time.Minute, server, Privileged(server, DeployCommand(&instance, config))); err != nil {
			return commandError("部署私网转发服务失败", output, err)
		}
	} else {
		if _, err := s.exec(ctx, 20*time.Second, server, Privileged(server, "systemctl is-active --quiet "+quote(Name(config.ID)+".service"))); err != nil {
			return fmt.Errorf("网关服务未运行，请重新部署")
		}
	}
	if err := log("正在创建或核对应用集群的 Service 和 EndpointSlice"); err != nil {
		return err
	}
	client, err := s.client(ctx, config.ClusterID)
	if err != nil {
		return err
	}
	if client == nil || client.Clientset == nil {
		return fmt.Errorf("集群客户端不可用")
	}
	if err := s.apply(ctx, client, config); err != nil {
		return err
	}
	if err := CleanupProbes(ctx, client, config); err != nil {
		return err
	}
	return s.probe(ctx, client, config, log)
}

func commandError(stage string, output []byte, failure error) error {
	text := strings.TrimSpace(string(output))
	if len(text) > 3000 {
		text = text[len(text)-3000:]
	}
	return fmt.Errorf("%s：%v %s", stage, failure, text)
}
