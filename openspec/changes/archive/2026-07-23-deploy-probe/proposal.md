## Why

当前部署 Agent 是"一键盲部署"——用户点击部署按钮后，系统直接 SSH 执行全流程，不检测远端是否已有 Agent。若远端已有运行中的 Agent，会静默覆盖，用户无感知。缺少前置探测和二次确认机制，容易误操作。

## What Changes

- 新增部署前探测接口 `POST /api/servers/:id/deploy/probe`，SSH 连接远端检查 Agent 进程、systemd service、二进制文件、版本号
- 改造部署接口 `POST /api/servers/:id/deploy` 支持 `?force=true` 覆盖模式
- Agent 二进制新增 `--version` 命令行参数
- 前端部署按钮改为"探测 → 确认 → 执行"三步交互

## Capabilities

### 新增能力

- `deploy-probe`: 部署前 Agent 状态探测，支持检测远端进程、systemd、二进制和版本

### 修改的能力

- `server-management`: 部署流程改为探测-确认-执行，新增 probe 端点
- `agent`: Agent 新增 `--version` flag

## 影响范围

- `internal/service/deployer/ssh_client.go` — 新增 `ProbeAgent()` 方法
- `internal/api/router.go` — `DeployService` 接口新增 `ProbeAgent`，`DeployAgent` 增加 force 参数
- `internal/api/server_handler.go` — 新增 `ProbeDeploy` handler，改造 `Deploy` handler
- `cmd/agent/main.go` — 新增 `--version` flag
- `web/src/views/Servers.vue` — 部署按钮改为探测 → 确认弹窗 → 执行
