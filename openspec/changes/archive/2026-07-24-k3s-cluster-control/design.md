## 技术方案

### 1. K8s 客户端初始化

**问题：** 当前 `NewInCluster()` 仅支持 Pod 内运行，开发环境不可用。

**方案：** 升级 `NewClient()` 为双模式：

```go
func NewClient() (*Client, error) {
    // 优先尝试 InCluster（生产）
    if cfg, err := rest.InClusterConfig(); err == nil {
        return newClientFromConfig(cfg)
    }
    // fallback: kubeconfig（开发/裸机）
    kubeconfig := os.Getenv("KUBECONFIG")
    if kubeconfig == "" {
        home, _ := os.UserHomeDir()
        kubeconfig = filepath.Join(home, ".kube", "config")
    }
    cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
    if err != nil {
        return nil, fmt.Errorf("no k8s config available")
    }
    return newClientFromConfig(cfg)
}
```

**K8s 不可用时的降级策略：**
- `main.go` 初始化失败不 panic，仅 log 警告
- 所有 K8s handler 在 client 为 nil 时返回 `{"error": "K8s 集群未连接", "data": []}`
- 前端通过 API 响应识别，显示"K8s 集群未连接"横幅

**全局注入：** `internal/api` 包级变量 `var K8s *k8s.Client`，`main.go` 中赋值。

### 2. IngressRoute 完整 CRUD

**API 设计：**

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/routes` | 列表（对接 K8s IngressRoute CRD） |
| POST | `/api/routes` | 创建（匹配规则 + 后端服务） |
| PUT | `/api/routes/:namespace/:name` | 更新 |
| DELETE | `/api/routes/:namespace/:name` | 删除 |

**Middleware 展示：**

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/routes/middlewares` | 列出所有 Traefik Middleware CRD |

**TLS Store 展示：**

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/routes/tls-stores` | 列出 TLS Store / TLS Options |

创建 IngressRoute 时，表单支持引用已有 Middleware 和 TLS Secret。

### 3. 集群 Dashboard

**API：**

```
GET /api/k8s/dashboard   → { nodes, namespaces, pods_total, pods_ready, version }
GET /api/k8s/pods         → Pod 列表，支持 ?namespace=xxx 过滤
GET /api/k8s/services     → Service 列表（含 ClusterIP、端口映射）
GET /api/k8s/deployments  → Deployment 列表（副本数、镜像）
```

### 4. Service 管理

**API：**

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/k8s/services/:namespace/:name` | 详情（ClusterIP、端口、Selector、关联 Pod） |
| PUT | `/api/k8s/services/:namespace/:name` | 更新端口映射、Selector |
| DELETE | `/api/k8s/services/:namespace/:name` | 删除 |

### 5. 前端改动

| 页面 | 变更 |
|------|------|
| Dashboard | 新增 K3s 集群状态卡片（节点/Pod/版本） |
| 路由 | IngressRoute 表单增加 Middleware + TLS 选择 |
| 路由 → Middleware Tab | 展示 Middleware 列表 |
| 新增：资源浏览 | Pods / Services / Deployments 三 Tab 列表页 |
| 服务器 → 集群节点 Tab | 对接真实 K8s Node API |
| 证书 | 对接真实 K8s Certificate CRD API |

### 6. 新增文件

| 文件 | 说明 |
|------|------|
| `internal/k8s/client.go` | 改造 NewClient（InCluster + kubeconfig） |
| `internal/k8s/dashboard.go` | ClusterSummary + ListPods/Services/Deployments |
| `internal/k8s/middleware.go` | Traefik Middleware CRD 操作 |
| `internal/k8s/tls_store.go` | Traefik TLS Store/TLS Option CRD 操作 |
| `internal/api/k8s_handler.go` | Dashboard/Pods/Services/Deployments handler |
| `internal/api/node_handler.go` | 改造，对接真实 K8s + SSH |
| `internal/api/ingress_handler.go` | 改造，完整 CRUD |
| `internal/api/cert_handler.go` | 改造，对接真实 K8s |
| `internal/api/k8s_handler_test.go` | handler 测试 |
| `internal/k8s/dashboard_test.go` | K8s 层测试（fake clientset） |
| `web/src/views/Resources.vue` | Pods/Services/Deployments 多 Tab 页面 |
| `web/src/views/Dashboard.vue` | 新增 K3s 状态卡片 |
| `web/src/views/Sites.vue` | Middleware 选择器 |
