## Why

当前平台已经能分别管理工作负载、Service、配置、路由和证书，但一次服务上线仍需要用户跨多个资源页手工协调，缺少发布版本、依赖校验、步骤进度和完整回滚。应用发布中心将这些 Kubernetes 资源组织成可审计的服务交付流程，补齐从镜像到 HTTPS 入口的日常运维主路径。

## What Changes

- 新增项目、环境、应用和发布版本的元数据模型，建立平台应用对象与 Kubernetes 资源的关联。
- 新增无状态 HTTP 服务发布流程：预检、创建/更新 ConfigMap 和 Secret、Deployment、ClusterIP Service、可选 Certificate 与 Ingress/IngressRoute，并等待资源就绪。
- 新增发布历史、步骤进度、失败重试和按上一成功版本回滚能力。
- 新增应用列表、详情和多步骤发布向导；现有资源页保留，并支持跳转到所属应用。
- 更新导航以提供“应用”日常入口，并更新操作日志以记录发布生命周期。
- **BREAKING**：平台发布中心只接管带 `app.kubernetes.io/managed-by=cylism-manager` 标签的资源；遇到同名非托管资源时阻断发布，不再静默覆盖。

## Capabilities

### New Capabilities

- `application-release`: 项目/环境/应用、发布预检、Kubernetes 资源编排、状态跟踪和回滚。

### Modified Capabilities

- `ui-navigation`: 增加应用发布中心入口，并保持基础设施资源页作为高级管理入口。
- `operation-log`: 将应用发布、重试与回滚纳入长流程操作日志和审计关联。

## Impact

- 后端新增 `internal/application` 领域服务、应用相关 Gin handler、GORM 模型与 SQLite 迁移，扩展 `internal/k8s` 的受控资源 apply/readiness 操作。
- 前端新增应用列表、详情和发布向导页面，更新 `App.vue` 导航和路由注册。
- Kubernetes RBAC 需要补齐 Deployment、Service、ConfigMap、Secret、Ingress、Certificate 的 create/update/patch/delete 权限；平台不新增外部依赖。
- 现有 `/workloads`、`/services`、`/configs`、`/routes`、`/certs` API 保持可用，作为应用资源的诊断与高级管理入口。
