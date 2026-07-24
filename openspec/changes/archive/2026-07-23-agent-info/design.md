## 背景

Server 表现有字段侧重 SSH 连接信息和 gRPC 端口，缺少 Agent 版本、部署路径、部署时间等元信息。HeartbeatMonitor 只检测 online 状态的 Agent，offline 后不再探测，缺少手动恢复到 online 的入口。

## 目标 / 非目标

**目标：**
- Server 表记录 Agent 版本、部署路径、部署时间
- 部署成功自动写入 Agent 信息
- 手动"状态同步"可探测 Agent 运行状态并将 offline 恢复为 online
- 前端 Agent 信息卡片展示

**非目标：**
- Agent 自动升级
- 多版本 Agent 管理

## 技术决策

### 1. 字段扩展在 Server 表

在 Server 表直接加 3 个可空字段，而非独立 Agent 表。当前一台服务器一个 Agent，无需外键解耦。

### 2. 状态同步复用 probe 接口

`POST /api/servers/:id/deploy/probe` 原本只返回探测结果，不写 DB。改造后在 handler 层：若探测成功且 process_running=true，自动更新 Server 状态。

```
ProbeDeploy handler:
  → h.deploySvc.ProbeAgent(server) 返回 AgentProbeResult
  → if result.ProcessRunning && server.Status != "online":
      server.Status = "online"
      server.AgentVersion = result.AgentVersion
      h.store.UpdateServer(server)
  → return result
```

### 3. 部署成功时写入

`DeployAgent` 成功后在 handler 层写入：

```
server.AgentVersion = version 常量
server.AgentDeployPath = "/opt/cylism-manager/agent"
server.AgentDeployedAt = &now
```

### 4. Agent 版本常量

在 `internal/api/router.go` 中已有一个硬编码的 version（在 cmd/agent/main.go 中定义为 "1.0.0"）。deploy 时直接用它。

## 风险与权衡

- **[offline→online 仅手动]** 无自动恢复机制，需用户手动触发状态同步 → 有意为之，避免误判
