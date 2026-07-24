## Why

Server 表目前不记录 Agent 版本、部署路径、部署时间等关键信息。已部署的 Agent 一旦被标记为 offline，心跳也不再检查，无法自动恢复 online。缺少一个手动触发"状态同步"的入口。

## What Changes

- Server 表新增 `agent_version`、`agent_deploy_path`、`agent_deployed_at` 字段
- 部署成功时自动写入 Agent 版本、路径、部署时间
- 状态同步（复用 probe 接口）探测到 Agent 运行时，自动更新 status=online + agent_version
- 前端新增"状态同步"按钮 + Agent 信息卡片

## Capabilities

### 新增能力

- `agent-info`: Agent 元信息记录与动态同步

### 修改的能力

- `server-management`: Server 表扩展 + Agent 状态同步 + Agent 信息展示

## 影响范围

- `internal/model/models.go` — Server 新增 3 字段
- `internal/api/router.go` — DeployAgent 写入 agent 信息；ProbeAgent 成功时更新 online 状态
- `web/src/views/Servers.vue` — 新增"状态同步"按钮 + Agent 信息卡片
