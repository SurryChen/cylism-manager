## 实现任务

### 1. K8s 客户端初始化
- [x] 改造 `internal/k8s/client.go`：NewClient() 支持 InCluster + K8S_API_HOST fallback
- [x] `main.go` 初始化 K8s client，注入 `api.K8s`，失败不 panic
- [x] `internal/api/router.go` 传递 K8s client 给 handler
- [x] 所有 K8s handler 添加 nil 检查，返回降级响应
- [x] 编写测试 + 编译通过

### 2. IngressRoute 完整 CRUD
- [x] IngressHandler 对接真实 K8s API（ListRoutes/DeleteRoute，Create/Update 预留）
- [x] 编写 handler 测试

### 3. Traefik Middleware + TLS Store
- [x] API 路由：GET `/api/routes/middlewares`，GET `/api/routes/tls-stores`
- [x] handler stub 实现

### 4. 集群 Dashboard
- [x] 新增 `internal/api/k8s_handler.go` — Dashboard/Pods/Services/Deployments handler
- [x] 编写 handler 测试

### 5. Node handler 真实对接
- [x] 改造 `internal/api/node_handler.go` 对接真实 K8s（List/Drain/Remove）
- [x] 编写 node handler 测试

### 6. Cert handler 真实对接
- [x] 改造 `internal/api/cert_handler.go` 对接真实 K8s（List/Delete）
- [x] 编写 cert handler 测试

### 7. Service ClusterIP 管理
- [x] GetService/DeleteService handler 实现
- [x] UpdateService handler 预留

### 8. 前端重构 + 主题切换 + UI 优化
- [x] 新增 Resources.vue（Pods/Services/Deployments 三 Tab）
- [x] Dashboard.vue 新增 K3s 集群状态卡片
- [x] App.vue 实现亮色/暗色主题切换（CSS 变量 + localStorage）
- [x] 全局 Design Tokens 重构（统一的 CSS 变量体系）
- [x] UI 优化：间距、层次、空状态、响应式
- [x] router/index.js 新增 /resources 路由

### 9. 全量验证
- [x] `go test -v -count=1 ./...` 全部通过（43 PASS, 0 FAIL）
- [x] `go build ./...` 编译通过
- [x] `cd web && npm run build` 前端构建通过
