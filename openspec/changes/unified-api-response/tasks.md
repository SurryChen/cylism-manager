## 1. 后端：统一响应结构体

- [ ] 1.1 创建 `internal/model/response.go`，定义 `APIResponse` 结构体、`Success()`、`Error()` helper 函数、错误码常量
- [ ] 1.2 编写 `internal/model/response_test.go` 单元测试

## 2. 后端：Handler 迁移

- [ ] 2.1 迁移 `k8s_handler.go`（~34 处 c.JSON → Success/Error）
- [ ] 2.2 迁移 `server_handler.go`（~8 处）
- [ ] 2.3 迁移 `site_handler.go`（~10 处）
- [ ] 2.4 迁移 `cert_handler.go`（~3 处）
- [ ] 2.5 迁移 `node_handler.go`（~5 处）
- [ ] 2.6 迁移 `ingress_handler.go`（~5 处）
- [ ] 2.7 迁移 `dashboard_handler.go`（~2 处）
- [ ] 2.8 迁移 `audit_handler.go`（~2 处）
- [ ] 2.9 迁移 `db_admin_handler.go`（~17 处）
- [ ] 2.10 迁移 `auth_handler.go` + `auth_middleware.go`（~12 处）
- [ ] 2.11 迁移 `crd_handler.go`（~1 处）
- [ ] 2.12 迁移 `operation_handler.go`（~2 处）

## 3. 前端：API 层重构

- [ ] 3.1 修改 `web/src/api/index.js`，增加 `_unwrapResponse()` 统一解包，修改 GET/POST/PATCH/DELETE 方法返回 data
- [ ] 3.2 添加前端 API 层测试

## 4. 前端：组件适配

- [ ] 4.1 适配 Dashboard.vue（移除 `.data` 中间访问）
- [ ] 4.2 适配 Servers.vue
- [ ] 4.3 适配 Sites.vue
- [ ] 4.4 适配 Certificates.vue
- [ ] 4.5 适配 Resources.vue
- [ ] 4.6 适配 AuditLogs.vue
- [ ] 4.7 适配 DBAdmin.vue
- [ ] 4.8 适配 Workloads.vue
- [ ] 4.9 适配 Services.vue
- [ ] 4.10 适配 Configs.vue
- [ ] 4.11 适配 Login.vue（认证接口特殊处理）

## 5. 全量验证

- [ ] 5.1 `go test ./...` 全部通过
- [ ] 5.2 `go build ./...` 编译通过
- [ ] 5.3 `npx vitest run` 前端测试通过
- [ ] 5.4 `npx vite build` CSS Design Token 检查通过
