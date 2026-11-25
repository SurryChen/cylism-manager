package applicationapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
)

type namespaceMaintenanceMover interface {
	PlanApplicationNamespaceMaintenance(context.Context, k8s.ApplicationNamespaceMove, string) (*k8s.ApplicationNamespaceMaintenancePlan, error)
	CheckApplicationNamespaceMaintenance(context.Context, k8s.ApplicationNamespaceMove, *k8s.ApplicationNamespaceMaintenancePlan, string) error
	PrepareApplicationNamespaceMaintenance(context.Context, k8s.ApplicationNamespaceMove, *k8s.ApplicationNamespaceMaintenancePlan, string, k8s.NamespaceMoveCopy) error
	CopyApplicationNamespaceMaintenance(context.Context, k8s.ApplicationNamespaceMove, *k8s.ApplicationNamespaceMaintenancePlan, string, []model.ApplicationEndpoint, k8s.NamespaceMoveCopy) error
	RunApplicationNamespaceMaintenanceTarget(context.Context, k8s.ApplicationNamespaceMove, *k8s.ApplicationNamespaceMaintenancePlan, string, bool) (bool, error)
	RestoreApplicationNamespaceMaintenanceSource(context.Context, k8s.ApplicationNamespaceMove, *k8s.ApplicationNamespaceMaintenancePlan, string) error
	CleanupApplicationNamespaceMaintenance(context.Context, k8s.ApplicationNamespaceMove, *k8s.ApplicationNamespaceMaintenancePlan, string) error
	SwitchApplicationNamespaceMaintenanceRoutes(context.Context, k8s.ApplicationNamespaceMove, *k8s.ApplicationNamespaceMaintenancePlan, string, []model.ApplicationEndpoint, bool) error
	PrepareApplicationNamespaceCertificates(context.Context, k8s.ApplicationNamespaceMove, string, map[string]string, string) error
}

var namespaceMaintenanceWorkers sync.Map

type namespaceMaintenanceStatus struct {
	Required         bool     `json:"maintenance"`
	Running          bool     `json:"running"`
	Ready            bool     `json:"ready"`
	Phase            string   `json:"phase,omitempty"`
	Detail           string   `json:"detail,omitempty"`
	Node             string   `json:"node,omitempty"`
	Claims           []string `json:"claims"`
	ConfigMaps       []string `json:"config_maps"`
	Secrets          []string `json:"secrets"`
	ManualValidation bool     `json:"manual_validation"`
}

func maintenanceStatus(task *model.ApplicationNamespaceMaintenance, plan *k8s.ApplicationNamespaceMaintenancePlan) namespaceMaintenanceStatus {
	status := namespaceMaintenanceStatus{Required: true, Node: plan.Node, ConfigMaps: plan.ConfigMaps,
		Secrets: plan.Secrets, ManualValidation: plan.ManualValidation}
	for _, claim := range plan.Claims {
		status.Claims = append(status.Claims, claim.Name)
	}
	if task != nil {
		status.Phase, status.Detail = task.Phase, task.Detail
		_, status.Running = namespaceMaintenanceWorkers.Load(task.ApplicationID)
	}
	return status
}

func (h *ApplicationHandler) handleNamespaceMaintenance(c *gin.Context, app *model.Application, move model.ApplicationNamespaceMove, runtime k8s.ApplicationNamespaceMove, task *model.ApplicationNamespaceMaintenance, action string) {
	adapter, ok := h.kubernetes.(interface {
		namespaceMaintenance() namespaceMaintenanceMover
	})
	if !ok {
		apiShared.K8sUnavailable(c)
		return
	}
	mover := adapter.namespaceMaintenance()
	ctx := c.Request.Context()
	var plan *k8s.ApplicationNamespaceMaintenancePlan
	var marker string
	if task == nil {
		kind := app.WorkloadKind
		if kind == "" {
			kind = "deployment"
		}
		var err error
		plan, err = mover.PlanApplicationNamespaceMaintenance(ctx, runtime, kind)
		if err == nil {
			err = mover.CheckApplicationNamespaceMaintenance(ctx, runtime, plan, "")
		}
		if err == nil {
			err = mover.SwitchApplicationNamespaceMaintenanceRoutes(ctx, runtime, plan, "", app.Endpoints, false)
		}
		if err != nil {
			apiShared.Conflict(c, err.Error())
			return
		}
	} else {
		marker = strconv.FormatUint(uint64(task.ID), 10)
		if task.SourceEnvironmentID != move.SourceEnvironmentID || task.TargetEnvironmentID != move.TargetEnvironmentID || task.ReleaseID != move.ReleaseID || task.OwnerID != move.OwnerID {
			apiShared.Conflict(c, model.ErrApplicationNamespaceMaintenanceActive.Error())
			return
		}
		if err := json.Unmarshal([]byte(task.Plan), &plan); err != nil || plan == nil {
			apiShared.DBError(c, "读取迁移检查点失败")
			return
		}
	}
	status := maintenanceStatus(task, plan)
	if action == "check" {
		if task != nil && !status.Running && (task.Phase == model.NamespaceMoveStarting || task.Phase == model.NamespaceMoveSwitching || task.Phase == model.NamespaceMoveRouted) {
			ready, err := mover.RunApplicationNamespaceMaintenanceTarget(ctx, runtime, plan, marker, false)
			status.Ready = ready
			if err != nil {
				status.Detail = err.Error()
			}
		}
		apiShared.Success(c, status)
		return
	}
	if _, running := namespaceMaintenanceWorkers.LoadOrStore(app.ID, true); running {
		apiShared.Conflict(c, "迁移操作正在执行")
		return
	}
	queued := false
	defer func() {
		if !queued {
			namespaceMaintenanceWorkers.Delete(app.ID)
		}
	}()
	current, err := h.applications.GetApplicationNamespaceMaintenance(app.ID)
	if err != nil || (current == nil) != (task == nil) || (current != nil && (current.ID != task.ID || current.Phase != task.Phase)) {
		apiShared.Conflict(c, "迁移状态已变更，请刷新后重试")
		return
	}

	phase := status.Phase
	certificates := map[string]string{}
	if action != "restore" {
		for _, endpoint := range app.Endpoints {
			if !endpoint.TLSEnabled || !endpointUsesIngress(endpoint) {
				continue
			}
			domain, err := h.resources.GetManagedDomain(endpoint.DomainID)
			if err != nil || domain.CertificateName == "" || domain.TLSSecretName != endpoint.TLSSecretName {
				apiShared.Conflict(c, "入口证书配置已变更")
				return
			}
			certificates[domain.CertificateName] = domain.TLSSecretName
		}
	}
	if action == "adopt" && phase == model.NamespaceMoveRouted {
		ready, err := mover.RunApplicationNamespaceMaintenanceTarget(ctx, runtime, plan, marker, false)
		if err == nil && !ready {
			err = fmt.Errorf("目标应用或 Service 还未就绪")
		}
		if err == nil {
			err = mover.SwitchApplicationNamespaceMaintenanceRoutes(ctx, runtime, plan, marker, app.Endpoints, true)
		}
		if err == nil {
			err = mover.PrepareApplicationNamespaceCertificates(ctx, runtime, marker, certificates, "verify")
		}
		if err != nil {
			apiShared.Conflict(c, err.Error())
			return
		}
		move.MaintenanceID = task.ID
		if err := h.applications.AdoptApplicationNamespace(move); err != nil {
			apiShared.Conflict(c, fmt.Sprintf("无法接管目标环境: %v", err))
			return
		}
		apiShared.Success(c, gin.H{"adopted": true, "project_id": runtime.TargetProjectID, "environment_id": runtime.TargetEnvironmentID})
		return
	}

	var copyVolume k8s.NamespaceMoveCopy
	if action == "prepare_maintenance" || (action == "start" && phase != model.NamespaceMoveStarting) {
		copyVolume, err = h.namespaceMoveVolumeCopier(app.Environment.ClusterID, plan)
		if err != nil {
			apiShared.Conflict(c, err.Error())
			return
		}
	}
	switch {
	case action == "prepare_maintenance" && (phase == "" || phase == model.NamespaceMovePreparing):
		if err := mover.PrepareApplicationNamespaceCertificates(ctx, runtime, marker, certificates, "check"); err != nil {
			apiShared.Conflict(c, err.Error())
			return
		}
		if task == nil {
			raw, err := json.Marshal(plan)
			if err != nil {
				apiShared.InternalError(c, "无法保存迁移计划")
				return
			}
			task, err = h.applications.BeginApplicationNamespaceMaintenance(move, string(raw))
			if err != nil {
				apiShared.Conflict(c, err.Error())
				return
			}
			marker = strconv.FormatUint(uint64(task.ID), 10)
		}
	case action == "start" && (phase == model.NamespaceMovePrepared || phase == model.NamespaceMoveStopping || phase == model.NamespaceMoveStarting):
	case action == "switch" && (phase == model.NamespaceMoveStarting || phase == model.NamespaceMoveSwitching):
	case action == "restore" && (phase == model.NamespaceMoveStopping || phase == model.NamespaceMoveRestoring || (phase == model.NamespaceMoveStarting && len(plan.Claims) == 0)):
	case action == "cancel" && (phase == model.NamespaceMovePreparing || phase == model.NamespaceMovePrepared || phase == model.NamespaceMoveCanceling):
	default:
		apiShared.Conflict(c, "当前迁移阶段不支持此操作")
		return
	}
	status = maintenanceStatus(task, plan)
	queued = true
	go func() {
		defer namespaceMaintenanceWorkers.Delete(app.ID)
		h.runNamespaceMaintenance(context.WithoutCancel(ctx), mover, runtime, plan, marker, task, action, app.Endpoints, certificates, copyVolume)
	}()
	apiShared.Success(c, status)
}

func (h *ApplicationHandler) runNamespaceMaintenance(parent context.Context, mover namespaceMaintenanceMover, runtime k8s.ApplicationNamespaceMove, plan *k8s.ApplicationNamespaceMaintenancePlan, marker string, task *model.ApplicationNamespaceMaintenance, action string, endpoints []model.ApplicationEndpoint, certificates map[string]string, copyVolume k8s.NamespaceMoveCopy) {
	ctx, cancel := context.WithTimeout(parent, 35*time.Minute)
	defer cancel()
	save := func(phase, detail string) error {
		return h.applications.UpdateApplicationNamespaceMaintenance(task, phase, detail)
	}
	recordFailure := func(err error) {
		_ = save(task.Phase, err.Error())
	}
	switch action {
	case "prepare_maintenance":
		if err := mover.PrepareApplicationNamespaceMaintenance(ctx, runtime, plan, marker, copyVolume); err != nil {
			recordFailure(err)
			return
		}
		if err := mover.PrepareApplicationNamespaceCertificates(ctx, runtime, marker, certificates, "prepare"); err != nil {
			recordFailure(err)
			return
		}
		_ = save(model.NamespaceMovePrepared, "")
	case "start":
		if task.Phase != model.NamespaceMoveStarting {
			if err := mover.PrepareApplicationNamespaceCertificates(ctx, runtime, marker, certificates, "verify"); err != nil {
				recordFailure(err)
				return
			}
			if err := save(model.NamespaceMoveStopping, ""); err != nil {
				return
			}
			if err := mover.CopyApplicationNamespaceMaintenance(ctx, runtime, plan, marker, endpoints, copyVolume); err != nil {
				if save(model.NamespaceMoveRestoring, err.Error()) == nil {
					restoreCtx, stop := context.WithTimeout(parent, 3*time.Minute)
					defer stop()
					if restoreErr := mover.RestoreApplicationNamespaceMaintenanceSource(restoreCtx, runtime, plan, marker); restoreErr != nil {
						recordFailure(fmt.Errorf("%v；恢复源副本失败: %w", err, restoreErr))
					} else {
						_ = save(model.NamespaceMovePrepared, fmt.Sprintf("%v；已恢复源副本", err))
					}
				}
				return
			}
			// Persist the point of no automatic return before the target can write.
			if err := save(model.NamespaceMoveStarting, ""); err != nil {
				return
			}
		}
		if _, err := mover.RunApplicationNamespaceMaintenanceTarget(ctx, runtime, plan, marker, true); err != nil {
			recordFailure(err)
			return
		}
		_ = save(model.NamespaceMoveStarting, "")
	case "switch":
		ready, err := mover.RunApplicationNamespaceMaintenanceTarget(ctx, runtime, plan, marker, false)
		if err != nil {
			recordFailure(err)
			return
		}
		if !ready {
			recordFailure(fmt.Errorf("目标工作负载或 Service 还未就绪"))
			return
		}
		if err := save(model.NamespaceMoveSwitching, ""); err != nil {
			return
		}
		if err := mover.SwitchApplicationNamespaceMaintenanceRoutes(ctx, runtime, plan, marker, endpoints, true); err != nil {
			recordFailure(err)
			return
		}
		_ = save(model.NamespaceMoveRouted, "")
	case "restore":
		if err := save(model.NamespaceMoveRestoring, task.Detail); err != nil {
			return
		}
		if err := mover.RestoreApplicationNamespaceMaintenanceSource(ctx, runtime, plan, marker); err != nil {
			recordFailure(err)
			return
		}
		_ = save(model.NamespaceMovePrepared, "已恢复源副本")
	case "cancel":
		if err := save(model.NamespaceMoveCanceling, ""); err != nil {
			return
		}
		if err := mover.PrepareApplicationNamespaceCertificates(ctx, runtime, marker, certificates, "cancel"); err != nil {
			recordFailure(err)
			return
		}
		if err := mover.CleanupApplicationNamespaceMaintenance(ctx, runtime, plan, marker); err != nil {
			recordFailure(err)
			return
		}
		if err := h.applications.CancelApplicationNamespaceMaintenance(task); err != nil {
			recordFailure(err)
		}
	}
}

// Serializes live application writes against migration creation and advancement.
func (h *ApplicationHandler) acquireApplicationRuntimeMutation(c *gin.Context, applicationID uint) (func(), bool) {
	if _, running := namespaceMaintenanceWorkers.LoadOrStore(applicationID, true); running {
		apiShared.Conflict(c, "应用已有运行环境操作正在执行")
		return nil, false
	}
	unlock := func() { namespaceMaintenanceWorkers.Delete(applicationID) }
	task, err := h.applications.GetApplicationNamespaceMaintenance(applicationID)
	if err != nil || task != nil {
		unlock()
		if err != nil {
			apiShared.DBError(c, "读取迁移状态失败")
		} else {
			apiShared.Conflict(c, model.ErrApplicationNamespaceMaintenanceActive.Error())
		}
		return nil, false
	}
	return unlock, true
}

func (h *ApplicationHandler) namespaceMoveVolumeCopier(clusterID uint, plan *k8s.ApplicationNamespaceMaintenancePlan) (k8s.NamespaceMoveCopy, error) {
	if len(plan.Claims) == 0 {
		return nil, nil
	}
	if h.serverAddresses == nil {
		return nil, fmt.Errorf("本地卷迁移需要注册源节点的 SSH 连接")
	}
	servers, err := h.serverAddresses.ListServers()
	if err != nil {
		return nil, fmt.Errorf("读取集群服务器失败")
	}
	var server *model.Server
	for i := range servers {
		candidate := &servers[i]
		if candidate.ClusterID != nil && *candidate.ClusterID == clusterID && candidate.K8sNodeName == plan.Node {
			if server != nil {
				return nil, fmt.Errorf("节点 %s 匹配了多个服务器", plan.Node)
			}
			server = candidate
		}
	}
	if server == nil {
		return nil, fmt.Errorf("节点 %s 暂无已注册的 SSH 连接", plan.Node)
	}
	return func(ctx context.Context, source, target string, copyData bool) error {
		return transport.SyncLocalVolumeContext(ctx, server, h.encKey, source, target, copyData)
	}, nil
}
