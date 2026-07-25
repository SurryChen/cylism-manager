## 1. 后端：统一响应结构体

- [x] 1.1 创建 `internal/model/response.go`，定义 `APIResponse` 结构体、`Success()`、`Error()` helper 函数、错误码常量
- [x] 1.2 编写 `internal/model/response_test.go` 单元测试

## 2. 后端：Handler 迁移

- [x] 2.1 迁移 `k8s_handler.go`（~34 处 c.JSON → Success/Error）
- [x] 2.2 迁移 `server_handler.go`（~8 处）
- [x] 2.3 迁移 `site_handler.go`（~10 处）
- [x] 2.4 迁移 `cert_handler.go`（~3 处）
- [x] 2.5 迁移 `node_handler.go`（~5 处）
- [x] 2.6 迁移 `ingress_handler.go`（~5 处）
- [x] 2.7 迁移 `dashboard_handler.go`（~2 处）
- [x] 2.8 迁移 `audit_handler.go`（~2 处）
- [x] 2.9 迁移 `db_admin_handler.go`（~17 处）
- [x] 2.10 迁移 `auth_handler.go` + `auth_middleware.go`（~12 处）
- [x] 2.11 迁移 `crd_handler.go`（~1 处）
- [x] 2.12 迁移 `operation_handler.go`（~2 处）

## 3. 前端：API 层重构

- [x] 3.1 修改 `web/src/api/index.js`，增加 `_unwrapResponse()` 统一解包，修改 GET/POST/PATCH/DELETE 方法返回 data
- [x] 3.2 添加前端 API 层测试

## 4. 前端：组件适配

- [x] 4.1 适配 Dashboard.vue（移除 `.data` 中间访问）
- [x] 4.2 适配 Servers.vue
- [x] 4.3 适配 Sites.vue
- [x] 4.4 适配 Certificates.vue
- [x] 4.5 适配 Resources.vue
- [x] 4.6 适配 AuditLogs.vue
- [x] 4.7 适配 DBAdmin.vue
- [x] 4.8 适配 Workloads.vue
- [x] 4.9 适配 Services.vue
- [x] 4.10 适配 Configs.vue
- [x] 4.11 适配 Login.vue（认证接口特殊处理）

## 5. 全量验证

- [x] 5.1 `go test ./...` 全部通过
- [x] 5.2 `go build ./...` 编译通过
- [x] 5.3 `npx vitest run` 前端测试通过
- [x] 5.4 `npx vite build` CSS Design Token 检查通过
