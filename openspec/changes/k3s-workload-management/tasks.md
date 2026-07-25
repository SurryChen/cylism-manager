## 1. 后端：工作负载 K8s 客户端封装

- [ ] 1.1 创建 `internal/k8s/workload.go`，实现 Deployment 列表/详情/关联 Pod/ReplicaSet 历史查询
- [ ] 1.2 实现 Deployment 扩缩容(PATCH replicas)、镜像更新(PATCH image)、回滚(POST rollback)
- [ ] 1.3 实现 StatefulSet 列表/详情/扩缩容
- [ ] 1.4 实现 DaemonSet 列表/详情
- [ ] 1.5 编写 `internal/k8s/workload_test.go` 单元测试

## 2. 后端：服务发现与 EndpointSlice

- [ ] 2.1 创建 `internal/k8s/service.go`，实现 Service 列表/详情（扩展 Endpoint 数量字段）
- [ ] 2.2 创建 `internal/k8s/endpointslice.go`，实现 EndpointSlice 查询（优先 discovery.k8s.io/v1，fallback v1/Endpoints）
- [ ] 2.3 编写 `internal/k8s/service_test.go` 和 `endpointslice_test.go`

## 3. 后端：配置管理

- [ ] 3.1 创建 `internal/k8s/config.go`，实现 ConfigMap 列表/详情及关联工作负载追踪
- [ ] 3.2 实现 Secret 列表（排 value）/详情（含 base64 value），关联工作负载追踪
- [ ] 3.3 编写 `internal/k8s/config_test.go`

## 4. 后端：标准 Ingress

- [ ] 4.1 创建 `internal/k8s/ingress_std.go`，实现标准 K8s Ingress 列表/详情/创建/删除
- [ ] 4.2 实现 Ingress Controller 检测（CRD + Deployment 状态）
- [ ] 4.3 编写 `internal/k8s/ingress_std_test.go`

## 5. 后端：API Handler 扩展

- [ ] 5.1 扩展 `internal/api/k8s_handler.go`，新增工作负载相关 handler 方法
- [ ] 5.2 新增 EndpointSlice handler 方法
- [ ] 5.3 新增 ConfigMap/Secret handler 方法
- [ ] 5.4 新增标准 Ingress handler 方法 + Ingress Controller 检测 handler
- [ ] 5.5 扩展 Dashboard handler，增加 deployments_total/deployments_ready/services_total
- [ ] 5.6 在 `internal/api/routes.go` 注册所有新路由
- [ ] 5.7 编写 `internal/api/k8s_handler_test.go`

## 6. 前端：工作负载页面

- [ ] 6.1 创建 `web/src/views/Workloads.vue`，实现 Deployment/StatefulSet/DaemonSet 三 Tab 切换
- [ ] 6.2 实现工作负载列表表格（名称、命名空间、副本数、镜像、年龄）
- [ ] 6.3 实现扩缩容弹窗（± 按钮 + 输入新副本数 + 确认）
- [ ] 6.4 实现更新镜像弹窗（选择容器 + 输入镜像 tag + 确认）
- [ ] 6.5 实现回滚弹窗（展示 ReplicaSet 历史 + 选择版本 + 确认）
- [ ] 6.6 实现关联 Pod 子面板（展开行显示 Pod 列表：名称、状态、节点、重启次数）
- [ ] 6.7 编写 `web/src/views/Workloads.test.js`

## 7. 前端：服务发现页面

- [ ] 7.1 创建 `web/src/views/Services.vue`，实现 Service 列表
- [ ] 7.2 实现 Service 展开面板：Selector → EndpointSlice → Endpoint 映射链路
- [ ] 7.3 Endpoint 状态着色（Ready/NotReady 不同颜色）
- [ ] 7.4 编写 `web/src/views/Services.test.js`

## 8. 前端：配置管理页面

- [ ] 8.1 创建 `web/src/views/Configs.vue`，实现 ConfigMap/Secret 双 Tab
- [ ] 8.2 实现 ConfigMap 列表 + 详情（键值对展示）
- [ ] 8.3 实现 Secret 列表 + 详情（默认遮蔽，点击眼睛切换显示/隐藏）
- [ ] 8.4 实现关联工作负载显示
- [ ] 8.5 编写 `web/src/views/Configs.test.js`

## 9. 前端：路由页扩展

- [ ] 9.1 扩展 `web/src/views/Sites.vue`，新增 IngressRoute/Ingress 双 Tab
- [ ] 9.2 实现标准 Ingress 列表（host、path → service:port）
- [ ] 9.3 实现 Ingress Controller 检测 Banner（调用 /api/system/crds + 展示 Traefik 状态）
- [ ] 9.4 编写 Sites.vue 扩展测试

## 10. 前端：Dashboard 增强 + 导航更新

- [ ] 10.1 扩展 Dashboard.vue 集群概况 Strip，增加 Deployment 和 Service 统计
- [ ] 10.2 更新 `web/src/App.vue` 导航：基础设施分组新增工作负载/服务发现/配置入口
- [ ] 10.3 更新 `web/src/router/index.js` 注册新路由
- [ ] 10.4 编写 Dashboard 扩展测试

## 11. 全量验证

- [ ] 11.1 `go test ./...` 全部通过
- [ ] 11.2 `go build ./...` 编译通过
- [ ] 11.3 `cd web && npm run test` 前端测试通过
- [ ] 11.4 `cd web && npx vite build` CSS Design Token 检查通过
