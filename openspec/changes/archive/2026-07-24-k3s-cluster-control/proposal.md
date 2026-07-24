## Why

`k3s-native-arch` 已将 Cylism Manager 切换到 K3s 原生架构，创建了 K8s 客户端封装、Node/IngressRoute/Certificate handler 骨架。但存在以下问题：

- K8s 客户端未在 `main.go` 中初始化，所有 handler 返回空数据（stub 实现）
- `NewInCluster()` 仅支持 Pod 内运行，开发环境不可用
- IngressRoute 缺少 Create/Update，Traefik Middleware 和 TLS Store 不可见
- 无集群资源总览（Pods/Services/Deployments）
- Node handler 只有 stub，未对接真实 K8s API 和 SSH 节点加入

## What Changes

- K8s 客户端初始化：InCluster + 环境变量 fallback（K8S_API_HOST/K8S_API_PORT），全局单例 `api.K8s`
- 所有 K8s handler（Node/Ingress/Cert）从 stub 改为真实 K8s API 调用
- 新增集群 Dashboard API（摘要 + Pods/Services/Deployments 列表）
- 新增 Service ClusterIP 管理（查看详情、删除）
- 前端新增"资源浏览"页面（Pods/Services/Deployments 多 Tab）
- Dashboard 新增 K3s 集群状态卡片
- 实现亮色/暗色主题切换（CSS 变量 + localStorage）
- 全局 Design Tokens 重构，UI 优化
