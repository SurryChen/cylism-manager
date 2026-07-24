## 实现任务

### 1. 新增 OperationLog 模型与数据库迁移
- [x] 在 `internal/model/models.go` 中新增 `OperationLog` 结构体（resource_type, resource_id, step, status, detail）
- [x] 在 `internal/store/store.go` 中新增 AutoMigrate、Create、ListByResource、DeleteExpired 方法
- [x] 为 OperationLog 模型编写单元测试（CRUD + 清理）

### 2. 新增操作日志配置段
- [x] 在 `config/config.yaml` 中新增 `operation_log.retention_days` 配置项（默认 30）
- [x] 在 `internal/config/config.go` 中新增对应结构体字段

### 3. 新增操作日志 API
- [x] 新增 `internal/api/operation_handler.go` — `ListOperations` handler（GET /api/operations?resource_type=X&resource_id=Y）
- [x] 在 `internal/api/router.go` 中注册路由
- [x] 编写 handler 单元测试

### 4. 部署服务接入操作日志
- [x] 在 `internal/api/router.go` 的 deploySvc 中注入 OperationLogStore，增加 LogCallback
- [x] 在部署流程各步骤（SSH 连接、检测 OS、上传二进制、启动 Agent）写入操作日志
- [x] 编写部署流程操作日志的集成测试

### 5. 新增后台清理任务
- [x] 在 `internal/api/router.go` 或 `main.go` 中启动后台 goroutine，每小时清理过期日志
- [x] 编写清理逻辑的单元测试

### 6. 前端操作日志组件与轮询
- [x] 在 `web/src/views/Servers.vue` 的详情面板中新增操作日志展示区域
- [x] 实现 2 秒轮询逻辑（有 running 时启动，全部完成或关闭面板时停止）
- [x] 日志条目显示步骤名称、状态图标（加载中/成功/失败）、详情和时间
- [x] 编写前端操作日志组件的测试（若已有测试框架）

### 7. 全量验证
- [x] `go test -v -count=1 ./...` 全部通过
- [x] `go build ./...` 编译通过
- [x] `cd web && npm run build` 前端构建通过
