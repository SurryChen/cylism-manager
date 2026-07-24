## Why

当前平台有 8 类长流程操作（部署 Agent、签发证书、续期证书、吊销证书、生成 NGINX 配置、重载 NGINX、导入 NGINX 配置、SSH 连通性测试），但用户触发后无法感知进度——不知道当前卡在哪一步、哪一步失败了。现有的 AuditLog 只记录操作结果（事后审计），缺少实时步骤追踪能力。

## What Changes

- 新增 `OperationLog` 通用操作日志模型（resource_type + resource_id 关联任意资源）
- 8 类长流程操作全部接入步骤日志（running → success/failed）
- 新增 `GET /api/operations` 通用查询接口（按资源类型+ID 筛选）
- 新增可配置的日志清理策略（默认保留 30 天）
- 前端服务器详情面板集成操作日志展示（实时轮询）
- 后续证书/站点操作的日志入口已预留

## 能力

### 新增能力

- `operation-log`: 通用操作日志记录与查询，支持实时步骤追踪、可配置清理策略

### 修改的能力

- `server-management`: 服务器详情面板新增操作日志展示
- `dashboard-audit`: 新增后台日志清理任务

## 影响范围

- 新增 `internal/model/models.go` — OperationLog 模型
- 新增 `internal/store/store.go` — OperationLog CRUD + 清理方法
- 修改 `internal/api/router.go` — deploySvc 增加日志回调 + 注册 operations 路由
- 修改 `internal/api/server_handler.go` — 新增 ListOperations handler（或新 handler）
- 新增 `internal/api/operation_handler.go` — 通用操作日志查询 handler
- 修改 `config/config.yaml` — 新增 operation_log 配置段
- 修改前端 `web/src/views/Servers.vue` — 详情面板 + 日志展示 + 轮询
