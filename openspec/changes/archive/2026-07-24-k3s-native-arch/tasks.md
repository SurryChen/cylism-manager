## 实现任务

### 1. K8s 客户端封装
- [x] 新增 `internal/k8s/client.go` — InClusterConfig + clientset + dynamic client
- [x] 新增 `internal/k8s/node.go` — Node List/Get/Drain/Delete
- [x] 新增 `internal/k8s/ingress.go` — IngressRoute CRUD（dynamic client）
- [x] 新增 `internal/k8s/cert.go` — Certificate CRUD
- [x] 编写 K8s client 单元测试（fake clientset）

### 2. 清理旧代码
- [x] 删除 `cmd/agent/`
- [x] 删除 `internal/agent/`
- [x] 删除 `internal/service/deployer/`
- [x] 删除 `internal/service/server/`
- [x] 删除 `api/proto/`
- [x] 删除 `internal/crypto/tls.go` 中部署相关函数
- [x] 清理 `internal/model/models.go` 中 Agent 相关字段
- [x] 清理 `internal/api/` 中 Agent/SSH 相关 handler
- [x] 清理 `config/config.yaml` 中 agent/tls 段
- [x] 编译通过，测试通过

### 3. 模型 + API 重构
- [x] Server 表新增 `cluster_role`、`k8s_node_name` 字段，保留 SSH 字段
- [x] 新增 `internal/api/node_handler.go` — ListNodes/AddNode/DrainNode/DeleteNode（操作 K8s API）
- [x] 新增 CRD 依赖检测：`internal/k8s/crd_check.go` — 启动时检测 Traefik/cert-manager CRD
- [x] 新增 `internal/api/ingress_handler.go` — ListRoutes/CreateRoute/DeleteRoute
- [x] 新增 `internal/api/cert_handler.go` — ListCerts/CreateCert/DeleteCert
- [x] 删除 Site/Cert 旧 handler
- [x] 保留 AuditLog/OperationLog/User handler
- [x] 编写 handler 测试

### 4. Platform 容器化
- [x] 新增 `Dockerfile`（多阶段构建，Go + Vue）
- [x] 新增 `k8s/platform-deployment.yaml`（含 PVC + Deployment + Service）
- [x] PVC 使用 local-path StorageClass，1Gi
- [x] 新增 `scripts/init-k3s.sh` — 初始化脚本（安装 K3s + cert-manager + deploy Platform）

### 5. 前端重构
- [x] 服务器页面：改成双 Tab（服务器列表 + 集群节点），移除 Agent/SSH 相关元素
- [x] 路由页面：Sites.vue 重写为 IngressRoute 管理
- [x] 证书页面：新增 Certificates.vue
- [x] 导航更新：站点→路由，新增证书，删除导入按钮

### 6. 全量验证
- [x] `go test -v -count=1 ./...` 全部通过（42 PASS, 0 FAIL）
- [x] `go build ./...` 编译通过
- [x] `cd web && npm run build` 前端构建通过
