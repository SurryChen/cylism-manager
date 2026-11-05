// Package migration coordinates platform-owned metric/log storage relocation.
// Durable checkpoints and shared cluster task logs survive browser/process exits.
package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
)

type Repository interface {
	ListServers() ([]model.Server, error)
	CreateObservabilityMigration(*model.ObservabilityMigration) error
	SaveObservabilityMigration(*model.ObservabilityMigration, string, bool) error
	ListObservabilityMigrations(uint, string) ([]model.ObservabilityMigration, error)
	ActiveObservabilityMigrations() ([]model.ObservabilityMigration, error)
	GetObservabilityMigration(uint) (*model.ObservabilityMigration, error)
	SetObservabilityMigrationControl(uint, string, string) error
	UpdateObservabilityMigrationProgress(uint, int64, int64, int64) error
}

type Adapter interface {
	ComponentStorageSnapshot(context.Context, string) (*k8s.ComponentStorageSnapshot, error)
	ValidateComponentMigrationNode(context.Context, string) error
	LockComponentMigration(context.Context, string, uint) error
	UnlockComponentMigration(context.Context, string, uint) error
	ProvisionComponentMigration(context.Context, *k8s.ComponentStorageSnapshot, string, string, uint) (*k8s.PersistentVolumeClaimInfo, error)
	SetComponentStorage(context.Context, *k8s.ComponentStorageSnapshot, string, string, int32, bool) error
	WaitComponentStorage(context.Context, *k8s.ComponentStorageSnapshot, string, string, bool) error
	DeletePVCBindingPodContext(context.Context, string, string) error
}

type Transfer interface {
	Exec(context.Context, *model.Server, string) ([]byte, error)
	Copy(context.Context, *model.Server, *model.Server, string, string, CopyControl) (int64, error)
}

type Service struct {
	repo     Repository
	adapter  func(context.Context, uint) (Adapter, error)
	transfer Transfer
	mu       sync.Mutex
	running  map[uint]*migrationRun
}

func New(repo Repository, factory func(context.Context, uint) (Adapter, error), transfer Transfer) *Service {
	return &Service{repo: repo, adapter: factory, transfer: transfer, running: map[uint]*migrationRun{}}
}

func (s *Service) List(clusterID uint, component string) ([]model.ObservabilityMigration, error) {
	items, err := s.repo.ListObservabilityMigrations(clusterID, component)
	for i := range items {
		s.capabilities(&items[i])
	}
	return items, err
}

func (s *Service) Start(ctx context.Context, clusterID uint, component, target string) (*model.ObservabilityMigration, error) {
	if clusterID == 0 {
		return nil, fmt.Errorf("请在指定集群的监控页面迁移组件")
	}
	if component != k8s.InfrastructureVictoriaMetrics && component != k8s.InfrastructureLoki {
		return nil, fmt.Errorf("不支持的监控组件")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("请选择目标节点")
	}
	a, err := s.adapter(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	snapshot, err := a.ComponentStorageSnapshot(ctx, component)
	if err != nil {
		return nil, err
	}
	if target == snapshot.Node {
		return nil, fmt.Errorf("目标节点与当前数据节点相同")
	}
	if err = a.ValidateComponentMigrationNode(ctx, target); err != nil {
		return nil, err
	}
	if _, _, err = s.servers(clusterID, snapshot.Node, target); err != nil {
		return nil, err
	}
	item := &model.ObservabilityMigration{ClusterID: clusterID, Component: component, SourceNode: snapshot.Node, TargetNode: target, SourcePVC: snapshot.PVC, Snapshot: k8s.EncodeComponentStorageSnapshot(snapshot), Status: "pending", Detail: "已创建迁移任务：" + snapshot.Node + " → " + target}
	if err = s.repo.CreateObservabilityMigration(item); err != nil {
		return nil, err
	}
	// Only immutable fields are returned; the worker operates on its own copy.
	s.launch(*item)
	return item, nil
}

func (s *Service) servers(clusterID uint, sourceNode, targetNode string) (*model.Server, *model.Server, error) {
	servers, err := s.repo.ListServers()
	if err != nil {
		return nil, nil, err
	}
	var source, target *model.Server
	for i := range servers {
		server := &servers[i]
		if server.ClusterID == nil || *server.ClusterID != clusterID {
			continue
		}
		if server.K8sNodeName == sourceNode {
			source = server
		}
		if server.K8sNodeName == targetNode {
			target = server
		}
	}
	if source == nil || target == nil {
		return nil, nil, fmt.Errorf("源节点和目标节点必须绑定到当前集群的平台注册服务器")
	}
	return source, target, nil
}

// Recover is called by the process-owned background loop. Interrupted writes
// are never replayed: restore the saved source before allowing another attempt.
func (s *Service) Recover(ctx context.Context) {
	items, err := s.repo.ActiveObservabilityMigrations()
	if err != nil {
		log.Printf("读取监控迁移恢复任务: %v", err)
		return
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		// A user-paused transfer must never be resumed or rolled back by polling.
		if item.Control == "pause" && (item.Status == "copying" || item.Status == "paused") {
			continue
		}
		s.launch(item)
	}
}

func (s *Service) launch(item model.ObservabilityMigration) {
	s.mu.Lock()
	if s.running[item.ID] != nil {
		s.mu.Unlock()
		return
	}
	run := newMigrationRun(item.Control)
	s.running[item.ID] = run
	s.mu.Unlock()
	go func() {
		defer func() { s.mu.Lock(); delete(s.running, item.ID); s.mu.Unlock() }()
		s.runControlled(&item, run)
	}()
}

func (s *Service) checkpoint(item *model.ObservabilityMigration, status, message string, terminal bool) error {
	for {
		s.mu.Lock()
		run := s.running[item.ID]
		// Honor a pause accepted after the last stream read, before leaving the
		// copying stage. Share the lock with Control so neither can bypass the other.
		if status == "verifying" && run != nil {
			run.mu.Lock()
			paused := run.control == "pause"
			run.mu.Unlock()
			if paused {
				s.mu.Unlock()
				if err := run.Wait(run.ctx); err != nil {
					return err
				}
				continue
			}
		}
		break
	}
	defer s.mu.Unlock()
	if run := s.running[item.ID]; status == "cutover" && run != nil && run.ctx.Err() != nil {
		return context.Cause(run.ctx)
	}
	previousStatus, previousDetail := item.Status, item.Detail
	item.Status, item.Detail = status, message
	if err := s.repo.SaveObservabilityMigration(item, message, terminal); err != nil {
		item.Status, item.Detail = previousStatus, previousDetail
		return err
	}
	return nil
}

func (s *Service) recordWaiting(item *model.ObservabilityMigration, message string) {
	if item.Detail == message {
		return
	}
	if err := s.checkpoint(item, item.Status, message, false); err != nil {
		log.Printf("迁移 #%d 记录等待状态失败: %v", item.ID, err)
	}
}

func (s *Service) run(item *model.ObservabilityMigration) {
	s.runControlled(item, newMigrationRun(item.Control))
}

func (s *Service) runControlled(item *model.ObservabilityMigration, run *migrationRun) {
	// The process-owned worker has no wall-clock deadline. Bound individual
	// readiness/connection waits and let the transfer detect missing progress.
	ctx := run.ctx
	defer run.cancel(nil)
	a, err := s.adapter(ctx, item.ClusterID)
	if err != nil {
		s.recordWaiting(item, "等待集群连接恢复："+err.Error())
		return
	}
	var snapshot k8s.ComponentStorageSnapshot
	if err = json.Unmarshal([]byte(item.Snapshot), &snapshot); err != nil {
		_ = s.checkpoint(item, "rollback_failed", "迁移快照损坏，需要人工核对；未自动修改工作负载", false)
		return
	}
	if err = a.LockComponentMigration(ctx, item.Component, item.ID); err != nil {
		s.recordWaiting(item, "等待迁移锁："+err.Error())
		return
	}
	if item.Control == "cancel" {
		if item.Status == "pending" || item.Status == "preflight" || item.Status == "provisioning_target" {
			_ = a.DeletePVCBindingPodContext(ctx, "monitoring", "obs-"+strconv.FormatUint(uint64(item.ID), 10))
			_ = s.finish(a, item, "canceled", "已取消迁移，源组件未停止；目标卷保留")
		} else {
			s.rollback(a, item, &snapshot, fmt.Errorf("用户取消迁移"))
		}
		return
	}
	if item.Status == "finalizing" {
		if err := s.finish(a, item, "succeeded", "迁移成功，目标组件已就绪；原卷保留"); err != nil {
			log.Printf("迁移 #%d 等待完成记录恢复: %v", item.ID, err)
		}
		return
	}
	if item.Status == "preflight" || item.Status == "provisioning_target" {
		_ = a.DeletePVCBindingPodContext(ctx, "monitoring", "obs-"+strconv.FormatUint(uint64(item.ID), 10))
		_ = s.finish(a, item, "failed", "平台进程中断，源组件未停止；目标卷保留，可重新发起迁移")
		return
	}
	if item.Status != "pending" {
		// If cutover was already healthy, keep the new store; rolling it back
		// could discard data that arrived after the platform process stopped.
		if item.Status == "waiting_ready" || item.Status == "succeeded" {
			err = wait(ctx, a, &snapshot, item.TargetNode, item.TargetPVC, true)
			if err == nil {
				s.complete(a, item)
				return
			}
		}
		s.rollback(a, item, &snapshot, fmt.Errorf("平台进程中断，按持久化快照恢复"))
		return
	}
	if err = s.execute(ctx, a, item, &snapshot, run); err != nil {
		if ctx.Err() != nil {
			err = context.Cause(ctx)
		}
		if item.Status == "pending" || item.Status == "preflight" || item.Status == "provisioning_target" {
			_ = a.DeletePVCBindingPodContext(ctx, "monitoring", "obs-"+strconv.FormatUint(uint64(item.ID), 10))
			s.finish(a, item, "failed", "迁移失败，源组件未停止："+err.Error()+"；已创建的目标卷保留供检查")
		} else {
			s.rollback(a, item, &snapshot, err)
		}
	}
}

func (s *Service) execute(ctx context.Context, a Adapter, item *model.ObservabilityMigration, snapshot *k8s.ComponentStorageSnapshot, run *migrationRun) error {
	if err := s.checkpoint(item, "preflight", "正在检查源/目标节点、SSH 与存储目录", false); err != nil {
		return err
	}
	if err := a.ValidateComponentMigrationNode(ctx, item.TargetNode); err != nil {
		return err
	}
	source, target, err := s.servers(item.ClusterID, item.SourceNode, item.TargetNode)
	if err != nil {
		return err
	}
	current, err := a.ComponentStorageSnapshot(ctx, item.Component)
	if err != nil {
		return err
	}
	if current.UID != snapshot.UID || current.PVC != snapshot.PVC || current.Node != snapshot.Node || current.Path != snapshot.Path {
		return fmt.Errorf("源工作负载或存储已变化，请重新发起迁移")
	}
	// Refresh the saved full spec immediately before stopping, so recovery does
	// not revert configuration that changed between request validation and locking.
	*snapshot = *current
	item.Snapshot = k8s.EncodeComponentStorageSnapshot(snapshot)
	for _, server := range []*model.Server{source, target} {
		if _, err = s.transfer.Exec(ctx, server, "sudo -n true && command -v tar && command -v sha256sum && command -v bash"); err != nil {
			return fmt.Errorf("节点 %s 的 SSH/sudo/复制工具预检失败: %w", server.Name, err)
		}
	}
	item.TargetPVC = fmt.Sprintf("cylism-%s-data-m%d", item.Component, item.ID)
	if err = s.checkpoint(item, "provisioning_target", "正在目标节点预配本地 PVC", false); err != nil {
		return err
	}
	claim, err := a.ProvisionComponentMigration(ctx, snapshot, item.TargetNode, item.TargetPVC, item.ID)
	if err != nil {
		return err
	}
	if err = s.checkDisk(ctx, source, target, snapshot.Path, claim.LocalPath); err != nil {
		return err
	}
	if err = s.checkpoint(item, "stopping_source", "正在停止源组件，等待所有写入退出", false); err != nil {
		return err
	}
	if err = a.SetComponentStorage(ctx, snapshot, snapshot.PVC, snapshot.Node, 0, false); err != nil {
		return err
	}
	if err = wait(ctx, a, snapshot, snapshot.Node, snapshot.PVC, false); err != nil {
		return err
	}
	if err = s.checkDisk(ctx, source, target, snapshot.Path, claim.LocalPath); err != nil {
		return err
	}
	sizeOut, err := s.transfer.Exec(ctx, source, "sudo -n du -sb -- "+quote(snapshot.Path))
	if err != nil {
		return err
	}
	item.BytesTotal, err = parseNumber(sizeOut)
	if err != nil {
		return err
	}
	run.onPause = func(paused bool) error {
		item.BytesCopied = run.copied.Load()
		if paused {
			item.Control = "pause"
			return s.checkpoint(item, "paused", "传输已暂停，连接和已复制数据保留；源服务保持停写，可继续或取消并恢复源服务", false)
		}
		item.Control = ""
		return s.checkpoint(item, "copying", "已继续传输，沿用当前连接和已传输数据", false)
	}
	if err = s.checkpoint(item, "copying", "正在流式复制数据，保留文件属主与权限", false); err != nil {
		return err
	}
	stopReporting := s.reportCopy(item, run)
	item.BytesCopied, err = s.transfer.Copy(ctx, source, target, snapshot.Path, claim.LocalPath, run)
	stopReporting()
	if err != nil {
		return err
	}
	if err = s.checkpoint(item, "verifying", fmt.Sprintf("已传输 %d 字节，正在校验完整目录 SHA-256", item.BytesCopied), false); err != nil {
		return err
	}
	if err = s.verify(ctx, source, target, snapshot.Path, claim.LocalPath); err != nil {
		return err
	}
	if err = s.checkpoint(item, "cutover", "校验通过，正在切换存储卷与调度节点", false); err != nil {
		return err
	}
	if err = a.DeletePVCBindingPodContext(ctx, "monitoring", "obs-"+strconv.FormatUint(uint64(item.ID), 10)); err != nil {
		return err
	}
	// Persist before enabling writes: recovery can recognize a completed cutover.
	if err = s.checkpoint(item, "waiting_ready", "正在启动并验证目标组件；Service 地址保持不变", false); err != nil {
		return err
	}
	if err = a.SetComponentStorage(ctx, snapshot, item.TargetPVC, item.TargetNode, 1, false); err != nil {
		return err
	}
	if err = wait(ctx, a, snapshot, item.TargetNode, item.TargetPVC, true); err != nil {
		return err
	}
	s.complete(a, item)
	return nil
}

// Once the target accepted writes, metadata failures must never trigger rollback.
// A durable waiting_ready/finalizing checkpoint lets the recovery loop finish.
func (s *Service) complete(a Adapter, item *model.ObservabilityMigration) {
	if err := s.checkpoint(item, "finalizing", "目标组件已就绪，正在完成迁移记录", false); err != nil {
		log.Printf("迁移 #%d 等待完成记录恢复: %v", item.ID, err)
		return
	}
	if err := s.finish(a, item, "succeeded", "迁移成功，目标组件已就绪；原卷保留，未删除历史数据"); err != nil {
		log.Printf("迁移 #%d 等待完成记录恢复: %v", item.ID, err)
	}
}

func wait(ctx context.Context, a Adapter, snapshot *k8s.ComponentStorageSnapshot, node, pvc string, ready bool) error {
	timeout := 5 * time.Minute
	if ready {
		// The destination may need its first image pull over the same slow link.
		timeout = 15 * time.Minute
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return a.WaitComponentStorage(waitCtx, snapshot, node, pvc, ready)
}

func (s *Service) rollback(a Adapter, item *model.ObservabilityMigration, snapshot *k8s.ComponentStorageSnapshot, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	if err := s.checkpoint(item, "rolling_back", "迁移未完成："+cause.Error()+"；正在停止目标并恢复源组件", false); err != nil {
		log.Printf("记录回退 #%d: %v", item.ID, err)
		return
	}
	err := a.SetComponentStorage(ctx, snapshot, snapshot.PVC, snapshot.Node, 0, true)
	if err == nil {
		err = wait(ctx, a, snapshot, snapshot.Node, snapshot.PVC, false)
	}
	if err == nil {
		err = a.DeletePVCBindingPodContext(ctx, "monitoring", "obs-"+strconv.FormatUint(uint64(item.ID), 10))
	}
	if err == nil {
		err = a.SetComponentStorage(ctx, snapshot, snapshot.PVC, snapshot.Node, 1, true)
	}
	if err == nil {
		err = wait(ctx, a, snapshot, snapshot.Node, snapshot.PVC, true)
	}
	if err != nil {
		_ = s.checkpoint(item, "rollback_failed", "恢复未完成，将自动重试；迁移锁保留："+err.Error(), false)
		return
	}
	_ = s.finish(a, item, "rolled_back", "迁移失败，已恢复源节点和源卷："+cause.Error()+"；目标卷保留供检查")
}

func (s *Service) finish(a Adapter, item *model.ObservabilityMigration, status, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// Unlock first. Until the terminal checkpoint is committed, restart recovery
	// still inspects the actual target readiness instead of replaying the copy.
	if err := a.UnlockComponentMigration(ctx, item.Component, item.ID); err != nil {
		return err
	}
	return s.checkpoint(item, status, message, true)
}

func (s *Service) checkDisk(ctx context.Context, source, target *model.Server, sourcePath, targetPath string) error {
	sizeOut, err := s.transfer.Exec(ctx, source, "sudo -n du -sb -- "+quote(sourcePath))
	if err != nil {
		return err
	}
	size, err := parseNumber(sizeOut)
	if err != nil {
		return err
	}
	// K3s storage parents may be root-only even when the PVC directory is writable.
	// Run every filesystem probe as root, including df's traversal of that path.
	out, err := s.transfer.Exec(ctx, target, "sudo -n test -d "+quote(targetPath)+" && test -z \"$(sudo -n find "+quote(targetPath)+" -mindepth 1 -maxdepth 1 -print -quit)\" && sudo -n df -B1 --output=avail -- "+quote(targetPath))
	if err != nil {
		return fmt.Errorf("目标目录非空或磁盘检查失败: %w；%s", err, strings.TrimSpace(string(out)))
	}
	available, err := parseNumber(out)
	if err != nil {
		return err
	}
	if available < size+size/5+128*1024*1024 {
		return fmt.Errorf("目标磁盘余量不足：需要数据大小外加 20%% 与 128 MiB 余量")
	}
	return nil
}

func (s *Service) verify(ctx context.Context, source, target *model.Server, sourcePath, targetPath string) error {
	checksum := func(server *model.Server, dir string) (string, error) {
		command := "cd " + quote(dir) + " && tar --sort=name --numeric-owner --mtime='UTC 1970-01-01' -cf - . | sha256sum"
		out, err := s.transfer.Exec(ctx, server, "sudo -n bash -o pipefail -c "+quote(command))
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && len(fields[0]) == 64 {
				return fields[0], nil
			}
		}
		return "", fmt.Errorf("未取得目录校验值")
	}
	left, err := checksum(source, sourcePath)
	if err != nil {
		return err
	}
	right, err := checksum(target, targetPath)
	if err != nil {
		return err
	}
	if left != right {
		return fmt.Errorf("源目录与目标目录校验不一致，取消切换")
	}
	return nil
}

func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func parseNumber(out []byte) (int64, error) {
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			if value, err := strconv.ParseInt(fields[0], 10, 64); err == nil && value >= 0 {
				return value, nil
			}
		}
	}
	return 0, fmt.Errorf("无法读取目录大小或磁盘可用空间")
}
