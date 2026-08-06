package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
)

// assistantRuntimeMigrationHooks isolate slow host and readiness operations in
// tests while production always uses the shared local-volume implementation.
type assistantRuntimeMigrationHooks struct {
	preflight func(*model.Server, string) error
	transfer  func(*model.Server, *model.Server, string, string, []byte) (int64, error)
	checksums func(*model.Server, string) (string, error)
	waitPods  func(string, string, bool, time.Duration) error
	waitReady func(time.Duration) error
}

func (h *AssistantHandler) migrationHooks() *assistantRuntimeMigrationHooks {
	return h.runtimeMigrationHooks
}

func (h *AssistantHandler) startAssistantRuntimeMigration(id uint) {
	h.migrationMu.Lock()
	if h.migrationRunning[id] {
		h.migrationMu.Unlock()
		return
	}
	h.migrationRunning[id] = true
	h.migrationMu.Unlock()

	go func() {
		defer func() {
			h.migrationMu.Lock()
			delete(h.migrationRunning, id)
			h.migrationMu.Unlock()
		}()
		h.runAssistantRuntimeMigration(id)
	}()
}

func (h *AssistantHandler) runAssistantRuntimeMigration(id uint) {
	migration, err := h.store.GetAssistantRuntimeMigration(id)
	if err != nil || K8s == nil {
		return
	}
	config := k8s.OpsAgentConfig{
		NodeName:         migration.TargetNodeName,
		Storage:          migration.Storage,
		StorageClassName: migration.StorageClassName,
		Image:            migration.Image,
		AuditPVCName:     migration.TargetPVCName,
	}
	if err := h.store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationProvisioning, "正在创建并绑定目标审计 PVC"); err != nil {
		return
	}
	if err := K8s.PrepareOpsAgentMigration(config, fmt.Sprint(migration.ID)); err != nil {
		h.failAssistantRuntimeMigration(migration, err, false)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	targetClaim, err := K8s.WaitForOpsAgentPVCBound(ctx, migration.TargetPVCName)
	cancel()
	if err != nil || !targetClaim.IsLocal || targetClaim.LocalPath == "" || targetClaim.BoundNode != migration.TargetNodeName {
		if err == nil {
			err = fmt.Errorf("目标审计 PVC 未绑定到所选节点")
		}
		h.failAssistantRuntimeMigration(migration, err, false)
		return
	}
	sourceClaim, err := K8s.OpsAgentPVCInfoByName(migration.SourceNamespace, migration.SourcePVCName)
	if err != nil || !sourceClaim.IsLocal || sourceClaim.LocalPath == "" || sourceClaim.BoundNode == "" {
		if err == nil {
			err = fmt.Errorf("旧审计 PVC 不是可迁移的 local-path/hostPath 卷")
		}
		h.failAssistantRuntimeMigration(migration, err, false)
		return
	}
	migration.SourceNodeName = sourceClaim.BoundNode
	if err := h.store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationStopping, "正在停止旧 Runtime 以获得一致的审计数据库"); err != nil {
		return
	}
	source, target, err := h.assistantMigrationServers(sourceClaim.BoundNode, targetClaim.BoundNode)
	if err == nil {
		err = h.preflightAuditTransfer(source, sourceClaim.LocalPath)
	}
	if err == nil {
		err = h.preflightAuditTransfer(target, targetClaim.LocalPath)
	}
	if err != nil {
		h.failAssistantRuntimeMigration(migration, err, false)
		return
	}
	if migration.SourceNamespace == migration.TargetNamespace {
		if err := K8s.ScaleOpsAgent(0); err != nil {
			h.failAssistantRuntimeMigration(migration, err, false)
			return
		}
		if err := h.waitForMigrationDeploymentPods(migration.SourceNamespace, "cylism-ops-agent", false, 90*time.Second); err != nil {
			h.failAssistantRuntimeMigration(migration, err, true)
			return
		}
	} else {
		legacyStatus := K8s.LegacyOpsAgentStatus()
		if legacyStatus.State != "not_installed" {
			if err := K8s.ScaleLegacyOpsAgent(0); err != nil {
				h.failAssistantRuntimeMigration(migration, err, false)
				return
			}
			if err := h.waitForMigrationDeploymentPods(migration.SourceNamespace, "cylism-ops-agent", false, 90*time.Second); err != nil {
				h.failAssistantRuntimeMigration(migration, err, true)
				return
			}
		}
	}
	if err := h.store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationCopying, "正在复制审计数据"); err != nil {
		return
	}
	bytesCopied, err := h.transferAssistantAuditData(source, target, sourceClaim.LocalPath, targetClaim.LocalPath)
	migration.BytesCopied = bytesCopied
	if err != nil {
		h.failAssistantRuntimeMigration(migration, err, true)
		return
	}
	if err := h.store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationVerifying, "正在校验审计 SQLite 文件"); err != nil {
		return
	}
	sourceChecksums, err := h.auditChecksums(source, sourceClaim.LocalPath)
	if err == nil {
		var targetChecksums string
		targetChecksums, err = h.auditChecksums(target, targetClaim.LocalPath)
		if err == nil && sourceChecksums != targetChecksums {
			err = fmt.Errorf("审计 SQLite 文件校验失败")
		}
	}
	if err != nil {
		h.failAssistantRuntimeMigration(migration, err, true)
		return
	}
	if err := K8s.DeleteOpsAgentMigrationBindingPod(fmt.Sprint(migration.ID)); err != nil {
		h.failAssistantRuntimeMigration(migration, err, true)
		return
	}
	if err := h.store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationStarting, "正在启动新的 Runtime"); err != nil {
		return
	}
	if migration.SourceNamespace == migration.TargetNamespace {
		if err := K8s.SwitchOpsAgentAuditPVC(migration.SourcePVCName, migration.TargetPVCName, migration.TargetNodeName, migration.SourceReplicas); err != nil {
			h.failAssistantRuntimeMigration(migration, err, true)
			return
		}
	} else {
		provider, err := h.store.GetAssistantProvider(migration.ProviderID)
		if err != nil || !provider.Enabled || provider.ProviderType != assistantProviderTypeResponses {
			if err == nil {
				err = fmt.Errorf("迁移使用的模型提供商不可用")
			}
			h.failAssistantRuntimeMigration(migration, err, true)
			return
		}
		apiKey, err := crypto.Decrypt(h.encKey, provider.APIKeyEncrypted)
		if err != nil {
			h.failAssistantRuntimeMigration(migration, fmt.Errorf("读取模型密钥失败: %w", err), true)
			return
		}
		config.Model, config.BaseURL = provider.Model, provider.BaseURL
		if _, err := K8s.InstallOpsAgent(config, apiKey); err != nil {
			h.failAssistantRuntimeMigration(migration, err, true)
			return
		}
	}
	if err := h.waitForMigratedOpsAgentReady(2 * time.Minute); err != nil {
		h.failAssistantRuntimeMigration(migration, err, true)
		return
	}
	if err := h.store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationCleaning, "新的 Runtime 已就绪，正在清理旧资源"); err != nil {
		return
	}
	if migration.SourceNamespace == migration.TargetNamespace {
		if err := K8s.DeleteOpsAgentAuditPVC(migration.SourcePVCName); err != nil {
			h.failAssistantRuntimeMigration(migration, err, false)
			return
		}
		_ = h.store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationSucceeded, "审计数据已迁移到目标节点，源 PVC 已清理")
		return
	}
	if err := K8s.CleanupLegacyOpsAgent(); err != nil {
		h.failAssistantRuntimeMigration(migration, err, false)
		return
	}
	_ = h.store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationSucceeded, "审计数据已迁移，旧 Runtime 资源已清理")
}

func (h *AssistantHandler) failAssistantRuntimeMigration(migration *model.AssistantRuntimeMigration, cause error, restoreSource bool) {
	if K8s != nil {
		_ = K8s.DeleteOpsAgentMigrationBindingPod(fmt.Sprint(migration.ID))
		if migration.SourceNamespace == migration.TargetNamespace {
			if restoreSource {
				_ = K8s.RestoreOpsAgentAuditPVC(migration.SourcePVCName, migration.TargetPVCName, migration.SourceNodeName, migration.SourceReplicas)
			}
			if migration.TargetPVCName != migration.SourcePVCName {
				_ = K8s.DeleteOpsAgentAuditPVC(migration.TargetPVCName)
			}
		} else if restoreSource {
			_ = K8s.ScaleOpsAgent(0)
			_ = K8s.ScaleLegacyOpsAgent(migration.SourceReplicas)
		}
	}
	_ = h.store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationFailed, cause.Error())
}

func (h *AssistantHandler) assistantMigrationServers(sourceNode, targetNode string) (*model.Server, *model.Server, error) {
	servers, err := h.store.ListServers()
	if err != nil {
		return nil, nil, err
	}
	var source, target *model.Server
	for index := range servers {
		if servers[index].K8sNodeName == sourceNode {
			source = &servers[index]
		}
		if servers[index].K8sNodeName == targetNode {
			target = &servers[index]
		}
	}
	if source == nil || target == nil || source.SSHAuthType != "key" || target.SSHAuthType != "key" {
		return nil, nil, fmt.Errorf("源节点和目标节点必须绑定使用 SSH 密钥认证的平台注册服务器")
	}
	return source, target, nil
}

func (h *AssistantHandler) preflightAuditTransfer(server *model.Server, path string) error {
	if hooks := h.migrationHooks(); hooks != nil && hooks.preflight != nil {
		return hooks.preflight(server, path)
	}
	command := "sudo -n true && command -v tar >/dev/null && command -v sha256sum >/dev/null && sudo -n test -d " + shellQuote(path)
	out, err := sshExec(20*time.Second, append(buildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return fmt.Errorf("服务器 %q 的迁移预检失败: %s", server.Name, strings.TrimSpace(string(out)))
	}
	return nil
}

func (h *AssistantHandler) transferAssistantAuditData(source, target *model.Server, sourcePath, targetPath string) (int64, error) {
	if hooks := h.migrationHooks(); hooks != nil && hooks.transfer != nil {
		return hooks.transfer(source, target, sourcePath, targetPath, h.encKey)
	}
	return streamPVCData(source, target, sourcePath, targetPath, h.encKey)
}

func (h *AssistantHandler) auditChecksums(server *model.Server, path string) (string, error) {
	if hooks := h.migrationHooks(); hooks != nil && hooks.checksums != nil {
		return hooks.checksums(server, path)
	}
	command := "for file in audit.db audit.db-wal audit.db-shm; do if sudo -n test -f " + shellQuote(path) + "/$file; then printf '%s:' \"$file\"; sudo -n sha256sum " + shellQuote(path) + "/$file | cut -d ' ' -f 1; fi; done"
	out, err := sshExec(30*time.Second, append(buildSSHArgs(server, h.encKey, server.Host), command))
	if err != nil {
		return "", fmt.Errorf("读取审计 SQLite 校验和失败: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (h *AssistantHandler) waitForOpsAgentReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if status := K8s.OpsAgentStatus(); status.State == "ready" {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("等待新的 Runtime 就绪超时")
}

func (h *AssistantHandler) waitForMigratedOpsAgentReady(timeout time.Duration) error {
	if hooks := h.migrationHooks(); hooks != nil && hooks.waitReady != nil {
		return hooks.waitReady(timeout)
	}
	return h.waitForOpsAgentReady(timeout)
}

func (h *AssistantHandler) waitForMigrationDeploymentPods(namespace, name string, ready bool, timeout time.Duration) error {
	if hooks := h.migrationHooks(); hooks != nil && hooks.waitPods != nil {
		return hooks.waitPods(namespace, name, ready, timeout)
	}
	return waitForDeploymentPods(namespace, name, ready, timeout)
}
