## 影响范围

### 新增 spec

**k8s-client-init：** K8s 客户端生命周期管理

- WHEN `main.go` 启动时，优先尝试 InClusterConfig，失败则 fallback 到 kubeconfig
- WHEN `main.go` 启动且 K8s 不可用时，log 警告但不 panic，设置 `K8s = nil`
- WHEN K8s handler 被调用且 `K8s == nil` 时，返回 `{"error": "K8s 集群未连接"}` 和空数据

**k8s-dashboard：** 集群资源总览

- WHEN 请求 `/api/k8s/dashboard` 时，返回节点总数、Pod 总数/就绪数、命名空间数、K3s 版本
- WHEN 请求 `/api/k8s/pods` 时，返回 Pod 列表（含命名空间、状态、节点、IP）
- WHEN 请求 `/api/k8s/services` 时，返回 Service 列表（含 ClusterIP、端口映射、类型）

**k8s-node-real：** 真实 K8s 节点管理

- WHEN 请求 `/api/nodes` 时，从 K8s API 获取节点列表（名称、状态、角色、版本、IP）
- WHEN 请求 `/api/nodes/:id/drain` 时，执行 cordon + 驱逐 Pod
- WHEN 请求 `DELETE /api/nodes/:id` 时，从集群中移除节点

**ingressroute-crud：** IngressRoute 完整 CRUD

- WHEN POST `/api/routes` 时，创建 Traefik IngressRoute CRD 资源
- WHEN PUT `/api/routes/:namespace/:name` 时，更新 IngressRoute 的 match 规则和 backend service

**traefik-middleware：** Middleware 展示

- WHEN 请求 `/api/routes/middlewares` 时，列出所有 Traefik Middleware 资源
- WHEN 列表返回时，显示 Middleware 名称、类型（RateLimit/RedirectRegex/BasicAuth）

### 更新 spec

**k3s-cert：** Certificate handler 从 stub 改为真实 K8s API

- WHEN 请求 `/api/certs` 时，从 cert-manager Certificate CRD 读取列表
- 属性更新：`status`（Ready/Issuing/Failed）、`domains`（从 dnsNames 解析）
