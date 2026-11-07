package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/database"
	"github.com/cylism/cylism-manager/internal/service/database/gateway"
	"github.com/cylism/cylism-manager/internal/store"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClientFactory func(context.Context, uint) (*k8s.Client, error)
type Service struct {
	store  *store.Store
	key    []byte
	client ClientFactory
	mu     sync.Mutex
	// Phase executor is replaceable in tests; production observes durable Jobs.
	execute func(context.Context, *model.DatabaseOperation, Request) (bool, string, error)
	cleanup func(context.Context, *model.DatabaseOperation) error
	settle  func(context.Context, *model.DatabaseOperation) (bool, error)
}

func New(s *store.Store, key []byte, client ClientFactory) *Service {
	service := &Service{store: s, key: append([]byte(nil), key...), client: client}
	service.execute = service.executePhase
	service.cleanup = service.cleanupJobs
	service.settle = service.settleReplication
	return service
}

var identifier = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)
var systemSchemas = map[string]bool{"mysql": true, "sys": true, "information_schema": true, "performance_schema": true}

func AssertUnlocked(tx *gorm.DB, refs ...string) error {
	var n int64
	if err := tx.Model(&model.DatabaseOperationLock{}).Where("resource IN ?", refs).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("资源仍被数据库任务占用，请先完成或终止任务")
	}
	return nil
}

func AssertNoReplication(tx *gorm.DB, ref string) error {
	var n int64
	if err := tx.Model(&model.DatabaseReplication{}).Where("source = ? OR target = ?", ref, ref).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("实例存在复制关系，请先解除复制")
	}
	return nil
}

func (s *Service) Queue(ctx context.Context, req Request) (*model.DatabaseOperation, error) {
	if len(Phases(req.Kind)) == 0 || req.Kind == "deploy" {
		return nil, fmt.Errorf("数据库操作类型无效")
	}
	if len(s.key) != 32 {
		return nil, fmt.Errorf("平台加密密钥未配置")
	}
	if req.ClusterID == 0 || strings.TrimSpace(req.NodeName) == "" {
		return nil, fmt.Errorf("请选择执行集群与节点；只检查该执行位置")
	}
	if req.Image == "" {
		req.Image = DefaultImage
	}
	if strings.ContainsAny(req.Image, " \t\r\n") {
		return nil, fmt.Errorf("执行镜像无效")
	}
	if (req.Kind == "migrate" || req.Kind == "replicate" || req.Kind == "replication_discover") && req.Source == "" {
		return nil, fmt.Errorf("请选择源数据库")
	}
	if req.Kind == "replication_discover" && req.Source == req.Target {
		return nil, fmt.Errorf("源与目标不能相同")
	}
	if req.Kind == "migrate" {
		if !req.WritesStopped {
			return nil, fmt.Errorf("快照迁移前请停止所选源数据库的业务写入，并保持停写直到应用切换完成")
		}
		if len(req.Schemas) == 0 || len(req.Schemas) > 100 {
			return nil, fmt.Errorf("请选择 1–100 个业务数据库")
		}
		seen := map[string]bool{}
		for _, name := range req.Schemas {
			if !identifier.MatchString(name) || systemSchemas[strings.ToLower(name)] || seen[name] {
				return nil, fmt.Errorf("数据库名称无效、重复或属于系统库")
			}
			seen[name] = true
		}
	}
	if req.Kind == "migrate" || req.Kind == "replicate" {
		if req.Source == req.Target {
			return nil, fmt.Errorf("源与目标不能相同")
		}
		if req.StorageSize == "" {
			req.StorageSize = "10Gi"
		}
		q, err := resource.ParseQuantity(req.StorageSize)
		if err != nil || q.Sign() <= 0 {
			return nil, fmt.Errorf("快照存储容量无效")
		}
	}
	if req.Kind == "replicate" && (!identifier.MatchString(req.ReplicationUser) || req.ReplicationPassword == "") {
		return nil, fmt.Errorf("请填写源库上已创建的复制账号和密码")
	}
	if req.Kind == "replicate" && len(req.ReplicationPassword) > 32 {
		return nil, fmt.Errorf("MySQL 复制账号密码不能超过 32 字节，请使用专用复制账号")
	}
	if req.Kind == "switch" && (len(req.Connections) == 0 || len(req.Connections) > 50) {
		return nil, fmt.Errorf("请选择 1–50 个已绑定的上线模板并填写目标业务连接")
	}
	if s.client == nil {
		return nil, fmt.Errorf("执行集群客户端未配置")
	}
	client, err := s.client(ctx, req.ClusterID)
	if err != nil {
		return nil, err
	}
	nodes, err := database.DeploymentNodes(ctx, client)
	if err != nil {
		return nil, err
	}
	allowed := false
	for _, n := range nodes {
		if n.Name == req.NodeName && n.DatabaseAllowed {
			allowed = true
		}
	}
	if !allowed {
		return nil, fmt.Errorf("执行节点未就绪或不允许调度数据库任务")
	}
	if req.Kind == "migrate" || req.Kind == "replicate" {
		req.StorageClass, err = database.ResolveStorageClass(ctx, client, req.StorageClass)
		if err != nil {
			return nil, err
		}
	}
	task := &model.DatabaseOperation{Kind: req.Kind, Source: req.Source, Target: req.Target, ClusterID: req.ClusterID, NodeName: req.NodeName, Status: "queued", PhaseName: Phases(req.Kind)[0]}
	err = s.store.DB().Transaction(func(tx *gorm.DB) error {
		input := jobInput{Kind: req.Kind, Schemas: req.Schemas, ReplicationUser: req.ReplicationUser, ReplicationPassword: req.ReplicationPassword}
		refs := []string{}
		if req.Source != "" && req.Kind != "detach" && req.Kind != "replication_check" {
			ep, e := s.resolve(ctx, tx, req.Source, req.ClusterID)
			if e != nil {
				return e
			}
			input.Source = ep
			refs = append(refs, req.Source)
		}
		if req.Kind == "inspect" && input.Source == nil {
			return fmt.Errorf("请选择要检查的实例")
		}
		if req.Kind != "inspect" {
			kind, id, e := ParseReference(req.Target)
			if e != nil || kind != "instance" {
				return fmt.Errorf("目标必须是已有的独立 MySQL 或外部实例")
			}
			var target model.DatabaseInstance
			if e = tx.First(&target, id).Error; e != nil {
				return e
			}
			if req.Kind == "replicate" || req.Kind == "detach" || req.Kind == "replication_check" || req.Kind == "replication_discover" {
				if target.Mode != "managed" || target.ClusterID != req.ClusterID {
					return fmt.Errorf("复制目标须为当前执行集群内的平台独立 MySQL，不能是 RDS 或 Operator 集群成员")
				}
			}
			if req.Kind == "replication_discover" {
				var relation model.DatabaseReplication
				e := tx.Where("target = ?", req.Target).First(&relation).Error
				if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
				if e == nil && relation.Source != req.Source {
					return fmt.Errorf("目标已登记其他源库的复制关系，请先核对")
				}
			}
			if req.Kind == "replicate" || req.Kind == "migrate" {
				if e = AssertNoReplication(tx, req.Target); e != nil {
					return e
				}
				var n int64
				if e = tx.Model(&model.DatabaseBinding{}).Where("instance_id = ?", id).Count(&n).Error; e != nil {
					return e
				}
				if n > 0 {
					return fmt.Errorf("目标仍被应用绑定，不能初始化或导入")
				}
			}
			if req.Kind == "detach" || req.Kind == "replication_check" {
				var relation model.DatabaseReplication
				if e = tx.Where("target = ?", req.Target).First(&relation).Error; e != nil {
					return fmt.Errorf("未找到目标的实际复制关系")
				}
				req.Source = relation.Source
				task.Source = relation.Source
				input.Channel = relation.Channel
				// Detach/check only touches the existing target, and does not require
				// the source to remain reachable or retain its administration password.
			}
			if req.Kind == "switch" {
				if e = AssertNoReplication(tx, req.Target); e != nil {
					return e
				}
				input.Connections = make([]SwitchConnection, len(req.Connections))
				seen := map[uint]bool{}
				for i, conn := range req.Connections {
					if seen[conn.BindingID] || !identifier.MatchString(conn.Database) || !identifier.MatchString(conn.Username) || conn.Password == "" {
						return fmt.Errorf("所选连接无效或重复")
					}
					seen[conn.BindingID] = true
					var binding model.DatabaseBinding
					var template model.ApplicationDeploymentTemplate
					if e = tx.First(&binding, conn.BindingID).Error; e != nil {
						return fmt.Errorf("应用绑定不存在")
					}
					if e = tx.First(&template, binding.TemplateID).Error; e != nil {
						return e
					}
					var app model.Application
					if e = tx.Preload("Environment").First(&app, binding.ApplicationID).Error; e != nil {
						return e
					}
					if target.Mode == "managed" && target.ClusterID != app.Environment.ClusterID {
						return fmt.Errorf("应用与目标 MySQL 不在同一集群")
					}
					if _, _, e = s.endpointAddress(tx, &target, app.Environment.ClusterID); e != nil {
						return e
					}
					conn.Revision = template.Revision
					input.Connections[i] = conn
					req.Connections[i].Revision = template.Revision
					req.Connections[i].Password = ""
					refs = append(refs, fmt.Sprintf("template:%d", template.ID))
				}
				// Each business credential is verified rather than using a root account.
				host, port, e := s.endpointAddress(tx, &target, req.ClusterID)
				if e != nil {
					return e
				}
				input.Target = &endpoint{Host: host, Port: port}
			} else {
				ep, e := s.resolve(ctx, tx, req.Target, req.ClusterID)
				if e != nil {
					return e
				}
				input.Target = ep
			}
			refs = append(refs, req.Target)
		}
		req.ReplicationPassword = ""
		req.ReplicationUser = ""
		for i := range req.Connections {
			req.Connections[i].Password = ""
		}
		raw, e := json.Marshal(req)
		if e != nil {
			return e
		}
		task.Spec = string(raw)
		if e = tx.Create(task).Error; e != nil {
			return e
		}
		if req.Kind == "replicate" {
			input.Channel = fmt.Sprintf("cylism_%d", task.ID)
		}
		raw, e = json.Marshal(input)
		if e != nil {
			return e
		}
		task.EncryptedInput, e = security.Encrypt(s.key, string(raw))
		if e != nil {
			return e
		}
		if e = tx.Save(task).Error; e != nil {
			return e
		}
		seen := map[string]bool{}
		for _, ref := range refs {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			if e = tx.Create(&model.DatabaseOperationLock{Resource: ref, TaskID: task.ID}).Error; e != nil {
				return fmt.Errorf("资源已有任务占用，请等待完成或终止原任务")
			}
		}
		return tx.Create(&model.DatabaseOperationLog{TaskID: task.ID, Message: "已创建独立数据库任务；暂停将在当前阶段结束后生效"}).Error
	})
	return task, err
}

func (s *Service) QueueDeployment(tx *gorm.DB, item *model.DatabaseInstance) error {
	req := Request{Kind: "deploy", Target: Reference(item.ID), ClusterID: item.ClusterID, NodeName: item.NodeName}
	raw, _ := json.Marshal(req)
	task := model.DatabaseOperation{Kind: "deploy", Target: req.Target, ClusterID: req.ClusterID, NodeName: req.NodeName, Spec: string(raw), Status: "queued", PhaseName: Phases("deploy")[0]}
	if err := tx.Create(&task).Error; err != nil {
		return err
	}
	if err := tx.Create(&model.DatabaseOperationLock{Resource: req.Target, TaskID: task.ID}).Error; err != nil {
		return err
	}
	return tx.Create(&model.DatabaseOperationLog{TaskID: task.ID, Message: "已创建独立 MySQL 部署任务"}).Error
}

func (s *Service) endpointAddress(tx *gorm.DB, item *model.DatabaseInstance, clusterID uint) (string, int, error) {
	if item.Mode == "managed" && item.ClusterID != clusterID {
		return "", 0, fmt.Errorf("独立 MySQL 当前仅支持同一 Kubernetes 集群内访问")
	}
	var gw model.DatabaseGateway
	err := tx.Where("instance_id = ?", item.ID).First(&gw).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if item.ConnectionMode == "gateway" {
			return "", 0, fmt.Errorf("请先配置私网网关")
		}
		return item.Host, item.Port, nil
	}
	if err != nil {
		return "", 0, err
	}
	// An optional diagnostic on another node must not block this operation.
	// A disabled, changing or pending gateway still cannot be used.
	if gw.ClusterID != clusterID || (gw.Status != "ready" && gw.Status != "degraded") {
		return "", 0, fmt.Errorf("私网网关未部署到当前执行集群或已停用")
	}
	var count int64
	if err = tx.Model(&model.DatabaseGatewayTask{}).Where("gateway_id = ? AND status IN ?", gw.ID, []string{"queued", "running"}).Count(&count).Error; err != nil {
		return "", 0, err
	}
	if count > 0 {
		return "", 0, fmt.Errorf("私网网关仍有变更任务")
	}
	return gateway.ServiceHost(gw.ID), 3306, nil
}

func (s *Service) resolve(ctx context.Context, tx *gorm.DB, ref string, clusterID uint) (*endpoint, error) {
	kind, id, err := ParseReference(ref)
	if err != nil {
		return nil, err
	}
	var encrypted string
	ep := &endpoint{}
	if kind == "cluster" {
		var cluster model.MySQLCluster
		if err = tx.First(&cluster, id).Error; err != nil {
			return nil, err
		}
		if cluster.ClusterID != clusterID || cluster.Status != "ready" {
			return nil, fmt.Errorf("源 Operator 集群须在执行集群内并处于运行状态")
		}
		client, e := s.client(ctx, clusterID)
		if e != nil {
			return nil, e
		}
		secret, e := client.Clientset.CoreV1().Secrets(cluster.Namespace).Get(ctx, cluster.ResourceName+"-root", metav1.GetOptions{})
		if e != nil {
			return nil, fmt.Errorf("读取源集群管理凭据失败")
		}
		ep.User = string(secret.Data["rootUser"])
		if ep.User == "" {
			ep.User = "root"
		}
		ep.Password = string(secret.Data["rootPassword"])
		ep.Host = cluster.ResourceName + "." + cluster.Namespace + ".svc.cluster.local"
		ep.Port = 3306
		if ep.Password == "" {
			return nil, fmt.Errorf("源集群缺少管理凭据")
		}
		return ep, nil
	}
	var item model.DatabaseInstance
	if err = tx.First(&item, id).Error; err != nil {
		return nil, err
	}
	ep.Host, ep.Port, err = s.endpointAddress(tx, &item, clusterID)
	if err != nil {
		return nil, err
	}
	if item.Mode == "managed" {
		ep.User = "root"
		encrypted = item.EncryptedRoot
	} else {
		ep.User = item.AdminUsername
		encrypted = item.EncryptedAdmin
	}
	if ep.User == "" || encrypted == "" {
		return nil, fmt.Errorf("请先配置实例管理账号（与业务账号分开）")
	}
	ep.Password, err = security.Decrypt(s.key, encrypted)
	if err != nil {
		return nil, fmt.Errorf("读取实例管理凭据失败")
	}
	return ep, nil
}

func (s *Service) Control(id uint, action string) error {
	return s.store.DB().Transaction(func(tx *gorm.DB) error {
		var task model.DatabaseOperation
		if err := tx.First(&task, id).Error; err != nil {
			return err
		}
		switch action {
		case "pause":
			if task.Status != "queued" && task.Status != "running" {
				return fmt.Errorf("当前任务不能暂停")
			}
			task.PauseRequested = true
		case "resume":
			if task.Status != "paused" {
				return fmt.Errorf("仅暂停的任务可继续；失败任务请查看日志后终止并处理目标")
			}
			task.PauseRequested = false
			task.Status = "queued"
		case "cancel":
			if task.Status == "succeeded" || task.Status == "cancelled" {
				return fmt.Errorf("任务已结束")
			}
			task.Status = "cancelling"
			task.PauseRequested = false
		default:
			return fmt.Errorf("操作无效")
		}
		if err := tx.Save(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.DatabaseOperationLog{TaskID: id, Message: map[string]string{"pause": "已请求暂停：当前阶段完成后暂停，不中断导入", "resume": "已继续任务", "cancel": "已请求终止；等待执行 Pod 退出后释放资源锁，保留目标数据和快照"}[action]}).Error
	})
}

func (s *Service) Reconcile(ctx context.Context) {
	if !s.mu.TryLock() {
		return
	}
	defer s.mu.Unlock()
	var tasks []model.DatabaseOperation
	if s.store.DB().Where("status IN ? OR cleanup_pending = ?", []string{"queued", "running", "cancelling"}, true).Order("id").Find(&tasks).Error != nil {
		return
	}
	for i := range tasks {
		if ctx.Err() != nil {
			return
		}
		s.reconcile(ctx, &tasks[i])
	}
}

func (s *Service) reconcile(ctx context.Context, task *model.DatabaseOperation) {
	if task.Status == "succeeded" || task.Status == "cancelled" {
		if err := s.cleanup(ctx, task); err == nil {
			_ = s.store.DB().Model(task).Update("cleanup_pending", false).Error
		}
		return
	}
	if task.Status == "cancelling" {
		if !task.CancellationSettled && ((task.Kind == "replicate" && task.Phase >= 3) || task.Kind == "detach") {
			done, err := s.settle(ctx, task)
			if err != nil || !done {
				s.progress(task, "终止中：等待执行 Pod 退出并核对实际复制通道；不可达时保留资源锁")
				return
			}
			if err := s.store.DB().Model(task).Update("cancellation_settled", true).Error; err != nil {
				return
			}
		}
		if err := s.cleanup(ctx, task); err != nil {
			s.progress(task, "等待执行 Pod 退出；若集群不可达将保留资源锁")
			return
		}
		message := "已终止；目标数据和迁移快照保留，复用目标前请检查部分导入或复制状态"
		if task.Kind == "deploy" {
			message = "已终止部署任务；已经创建的实例和数据卷保留，可刷新部署状态或重新部署"
		}
		if task.Kind == "switch" && task.ConnectionsApplied {
			message = "已终止任务；所选连接模板已经保存，请到应用页面核对并决定是否发布"
		}
		s.finish(task, "cancelled", message)
		return
	}
	var req Request
	if json.Unmarshal([]byte(task.Spec), &req) != nil {
		s.fail(task, "任务配置损坏")
		return
	}
	// Pause an unsubmitted phase immediately. A submitted Job is allowed to
	// reach its checkpoint; never kill an import just to implement a pause.
	if task.PauseRequested && task.Status == "queued" {
		s.pause(task)
		return
	}
	// Mark running before submission. Missing Jobs can only be submitted by the
	// deterministic phase executor; created Jobs are never automatically deleted.
	if task.Status == "queued" {
		update := s.store.DB().Model(task).Where("status = ? AND pause_requested = ?", "queued", false).Update("status", "running")
		if update.Error != nil || update.RowsAffected != 1 {
			return
		}
		task.Status = "running"
	}
	done, result, err := s.execute(ctx, task, req)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		s.fail(task, err.Error())
		return
	}
	if !done {
		if result != "" {
			s.progress(task, result)
		}
		return
	}
	// Update stage and relation atomically. Action requests arriving while a
	// phase runs are reloaded rather than overwritten by a stale task snapshot.
	err = s.store.DB().Transaction(func(tx *gorm.DB) error {
		var latest model.DatabaseOperation
		if e := tx.First(&latest, task.ID).Error; e != nil {
			return e
		}
		if latest.Status != "running" {
			return nil
		}
		if e := tx.Create(&model.DatabaseOperationLog{TaskID: task.ID, Message: latest.PhaseName + "：完成"}).Error; e != nil {
			return e
		}
		if latest.Kind == "replicate" && latest.Phase == 3 {
			var relation model.DatabaseReplication
			err := tx.Where("target = ?", latest.Target).First(&relation).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				relation = model.DatabaseReplication{Source: latest.Source, Target: latest.Target, TaskID: latest.ID, Channel: fmt.Sprintf("cylism_%d", latest.ID), Status: "checking"}
				if e := tx.Create(&relation).Error; e != nil {
					return e
				}
			} else if err != nil {
				return err
			}
		}
		latest.Phase++
		latest.Progress = ""
		latest.Result = result
		if latest.Phase >= len(Phases(latest.Kind)) {
			if latest.Kind == "replication_discover" || latest.Kind == "replication_check" || (latest.Kind == "replicate" && result != "") {
				if e := recordReplicationObservation(tx, &latest, result); e != nil {
					return e
				}
			}
			if latest.Kind == "replicate" {
				if e := tx.Model(&model.DatabaseReplication{}).Where("target = ? AND task_id = ?", latest.Target, latest.ID).Update("status", "running").Error; e != nil {
					return e
				}
			}
			if latest.Kind == "detach" {
				if e := tx.Where("target = ?", latest.Target).Delete(&model.DatabaseReplication{}).Error; e != nil {
					return e
				}
			}
			latest.Status = "succeeded"
			latest.PhaseName = "已完成"
			latest.EncryptedInput = ""
			latest.CleanupPending = true
			if e := tx.Where("task_id = ?", latest.ID).Delete(&model.DatabaseOperationLock{}).Error; e != nil {
				return e
			}
		} else {
			latest.PhaseName = Phases(latest.Kind)[latest.Phase]
			latest.Status = "queued"
			if latest.PauseRequested {
				latest.Status = "paused"
			}
		}
		return tx.Save(&latest).Error
	})
	if err != nil {
		s.fail(task, "记录阶段结果失败；请保留任务并检查数据库状态")
	}
	// Terminal tasks remain in reconciliation until all credential-bearing
	// execution resources are removed. Snapshot PVCs are deliberately retained.
}
func (s *Service) pause(task *model.DatabaseOperation) {
	_ = s.store.DB().Model(task).Where("status = ? AND pause_requested = ?", "queued", true).Updates(map[string]any{"status": "paused", "progress": "已在阶段检查点暂停"}).Error
}
func (s *Service) progress(task *model.DatabaseOperation, value string) {
	if task.Progress == value {
		return
	}
	_ = s.store.DB().Model(task).Update("progress", value).Error
}
func (s *Service) fail(task *model.DatabaseOperation, message string) {
	_ = s.store.DB().Transaction(func(tx *gorm.DB) error {
		update := tx.Model(task).Where("status IN ?", []string{"queued", "running"}).Updates(map[string]any{"status": "failed", "last_error": message})
		if update.Error != nil || update.RowsAffected != 1 {
			return update.Error
		}
		if task.Kind == "replication_check" || task.Kind == "replicate" {
			_ = tx.Model(&model.DatabaseReplication{}).Where("target = ?", task.Target).Update("status", "error").Error
		}
		if task.Kind == "deploy" {
			_, id, _ := ParseReference(task.Target)
			if err := tx.Model(&model.DatabaseInstance{}).Where("id = ?", id).Updates(map[string]any{"status": "error", "last_error": message}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.DatabaseOperationLog{TaskID: task.ID, Message: message + "；保留资源锁，请检查后终止此任务"}).Error
	})
}
func (s *Service) finish(task *model.DatabaseOperation, status, message string) {
	_ = s.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(task).Updates(map[string]any{"status": status, "progress": message, "encrypted_input": "", "cleanup_pending": true}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", task.ID).Delete(&model.DatabaseOperationLock{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.DatabaseOperationLog{TaskID: task.ID, Message: message}).Error
	})
}
