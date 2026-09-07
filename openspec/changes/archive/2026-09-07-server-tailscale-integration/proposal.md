## Why

当前服务器管理功能薄弱：
- 表格仅展示 5 列（名称/主机/SSH端口/集群角色/节点名），缺乏 SSH 用户、认证方式、连通性等运维核心信息
-「加入集群」操作为 mock 空壳（"SSH 集成待实现"），无法真正将服务器加入 k3s 集群
- 跨公网/内网的 k3s 节点间无稳定网络通道，NAT 后的内网机器无法与 control-plane 通信
- `Server.Status`/`Server.LastSeen` 字段从未被有效更新或使用，Dashboard 的 `online_servers` 数据不可靠

## What Changes

- 清理废弃字段：删除 `Server.Status`、`Server.LastSeen`、`DashboardStats.OnlineServers`
- 补充 `Server` 模型的 `TailscaleIP`/`TailscaleOnline` 字段
- 新增 `system_configs` 表，存储加密敏感配置（Tailscale Auth Key、k3s join token）
- 新增 `POST /api/tailscale/init` — 本机 Tailscale 初始化（一键安装+注册）
- 新增 `POST /api/servers/:id/probe` — SSH 连通性快检
- 新增 `POST /api/servers/:id/precheck` — 加入集群前置检测（root/swap/OS/磁盘）
- 新增 WebSocket `/api/nodes/:id/join-progress` — 加入集群 12 步实时进度日志
- 重写 `POST /api/nodes/:id/add` — 从 mock 改为完整流程：SSH + Tailscale 安装 + k3s-agent 安装 + 注册等待
- 前端 Servers.vue 表格增加 SSH 用户/认证方式/连通性/TS IP/TS 状态列
- 前端新增 ProgressModal 组件 + Tailscale 配置页面
- Dashboard 删除「在线」指标卡，新增 Tailscale 未初始化 Banner

## Capabilities

### New Capabilities

- `tailscale-integration`: Tailscale 初始化、状态查询、Auth Key 管理
- `server-ssh-probe`: SSH 连通性检测
- `server-precheck`: 加入集群前置条件检测
- `websocket-progress`: WebSocket 实时进度日志基础设施

### Modified Capabilities

- `server-management`: Server 模型字段变更、表格列变更、新增 probe/precheck 接口
- `dashboard-audit`: Dashboard 删除 OnlineServers、新增 Tailscale Banner
- `k3s-node`: AddNode 从 mock 改为完整实现

## Impact

- **后端**: 新增 `internal/api/tailscale_handler.go`、新增 `internal/model/system_config.go`、修改 `Server` 模型、修改 4 个 handler、新增 WebSocket 支持（gorilla/websocket）
- **前端**: 修改 `Servers.vue`（表格+弹窗）、修改 `Dashboard.vue`（指标卡）、新增 `TailscaleConfig.vue`、新增 `ProgressModal.vue`
- **数据库**: Server 表删除 2 列 + 新增 2 列、新增 system_configs 表
- **部署**: platform-deployment.yaml 新增 ENCRYPTION_KEY 环境变量
- **依赖**: 新增 `github.com/gorilla/websocket`（Go WebSocket 库）
- **兼容性**: **BREAKING** — Server API JSON 字段变更，前后端需同步部署
