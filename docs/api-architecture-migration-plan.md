# API Architecture Migration Plan

## 目标

逐步将 `internal/api` 从按功能堆叠的单一 package，演进为按领域组织、分层清晰、可被 Web UI、CLI、Agent 和后台任务复用的结构。

迁移过程中保持现有 REST API 路径和返回契约不变。每次只迁移一个完整功能模块，验证通过后再开始下一个模块。

## 目标目录

```text
internal/
  api/
    router.go
    middleware/
    auth/
    application/
    infrastructure/
    delivery/
    agent/
    system/
  service/
    application/
    registry/
    platform/
    cluster/
    storage/
    network/
    certificate/
  repository/
  domain/
  k8s/
  model/
  store/
```

说明：Git 不会记录空目录，因此不使用大量 `.gitkeep`。目录在对应模块开始迁移时创建，并在目录中放入实际代码或必要的 package 文档。

## 依赖规则

```text
HTTP Handler
    -> Service
        -> Repository
        -> K8s Adapter
        -> Domain / Model
```

### API 层

API Handler 只负责：

- HTTP 参数解析和输入校验
- 权限和请求上下文处理
- 调用 Service
- 将业务错误转换为统一 HTTP 响应

Handler 不直接编排数据库事务、复杂业务规则或 Kubernetes 资源生命周期。

### Service 层

Service 负责完整的业务用例，例如：

- 部署、修复和删除自托管制品库
- 创建和执行应用发布
- 配置节点镜像源
- 管理平台版本发布

Service 必须可以脱离 Gin Context 测试，并尽量不依赖 HTTP 类型。

### Repository 层

Repository 封装持久化访问。迁移初期可以通过现有 `store.Store` 提供实现，但业务 Service 不应继续扩散 GORM 查询细节。

### K8s 层

`internal/k8s` 负责 Kubernetes API 和资源适配。只有被多个领域复用的能力才抽取为稳定接口，避免为了目录整齐而过早拆分。

### 禁止的依赖

- `service` 不依赖 `api`
- `repository` 不依赖 `api`
- `k8s` 不依赖 `api`
- API 子包之间不互相调用业务逻辑
- 不在多个 Handler 中复制同一套业务校验或资源渲染逻辑

共享能力应下沉到 `service`、`domain` 或明确命名的基础包中。

## 迁移阶段

### 阶段 0：建立基线

实施状态：已完成（2026-08-30）。目录、复杂度和 Router 路由基线记录在 `docs/internal-architecture-stage0-baseline.md`；共享 API 辅助函数已收敛到 `internal/api/shared`，现有 REST 路径、鉴权和业务行为保持不变。

不改变业务行为，完成以下工作：

1. 记录当前 `internal/api` 文件、路由和主要依赖。
2. 标记超过 1000 行或同时包含 HTTP、业务和 Kubernetes 操作的文件。
3. 执行并记录：

   ```bash
   go test ./...
   go build ./...
   cd web && npm run build
   ```

4. 确认核心页面和 API 正常。
5. 以本文件作为后续迁移的边界约定。

### 阶段 1：自托管制品库

状态：已完成（2026-08-29）。

已落地内容：

- HTTP Handler 已迁移至 `internal/api/delivery`，路由、鉴权、错误码和响应结构保持不变。
- 制品库配置、凭据、关联镜像源和删除引用保护由 `internal/service/registry` 与 `internal/repository` 承担。
- PVC、StorageClass、Ready 节点、证书/TLS Secret、Kubernetes 资源 reconcile 和运行状态读取均收敛到 `internal/k8s`。
- 节点镜像源配置渲染与 SSH 下发通过 Registry Service 的适配器契约复用，制品库 Handler 不再调用节点镜像源 Handler。
- 已补充 Service、Reconciler 和 API 集成测试；全量验证结果记录在本次迁移提交中。

优先迁移 `managed OCI Registry`，因为它边界完整、耦合较高，并且后续 Registry 功能可以复用其服务层。

目标结构：

```text
internal/api/delivery/managed_registry_handler.go
internal/api/delivery/managed_registry_request.go

internal/service/registry/managed_registry_service.go
internal/service/registry/managed_registry_validator.go

internal/repository/registry_repository.go
internal/k8s/registry.go
```

保持以下 API 路径不变：

```text
/api/managed-oci-registries
/api/managed-oci-registries/storage-preflight
/api/managed-oci-registries/pvcs
/api/managed-oci-registries/certificates
```

### 阶段 2：提取 Registry 共用能力

状态：已完成（2026-08-29）。

已落地内容：

- 镜像仓库、节点镜像源和应用发布统一使用共享的镜像引用解析、Registry 前缀匹配和 Docker Hub 别名判断。
- CPU、内存 request/limit 与证书域名覆盖校验从受管制品库校验文件中拆出为独立资源校验能力。
- Registry 凭据的加密、解密和“已配置”状态判断统一收敛；节点 `registries.yaml` 仅在渲染下发前解密，避免各入口复制凭据处理循环。
- 外部 REST 路径、请求字段、响应字段和既有中文错误文案保持不变。

本阶段实现文件：

```text
internal/service/registry/image_reference.go
internal/service/registry/resource_validation.go
internal/service/registry/registry_credentials.go
internal/service/registry/node_mirror.go
```

统一处理：

- 镜像名称和 Registry 前缀解析
- endpoint 校验
- CPU、内存和 PVC 校验
- Ingress/TLS 配置
- 节点镜像源配置
- 凭据保存和脱敏

### 阶段 3：Registry Proxy 和节点镜像源

状态：已完成（2026-08-29）。

已落地内容：

- 节点镜像源的输入校验、凭据加密、连接检测状态、节点筛选与单飞异步下发任务收敛至 `internal/service/registry/mirror_service.go`；API 层仅保留 HTTP 映射和 SSH 适配。
- 镜像代理的请求校验、默认值、NodePort 冲突检测、出网代理凭据和持久化收敛至 `internal/service/registry/proxy_service.go`。
- 镜像代理的 Kubernetes Deployment、Service、节点检查、缓存清理和状态读取收敛至 `internal/k8s/registry_proxy.go`，K8s 层只接收服务层解密后的运行环境变量。
- Registry Proxy 与节点镜像源的 HTTP Adapter 已迁移至 `internal/api/delivery/`；路由、鉴权、错误码和响应字段保持不变。
- Registry Proxy 的 Ready Pod 查询、固定诊断命令、Pod exec 和诊断结果解析已迁移至 `internal/k8s/registry_proxy.go`；API 层不再执行 Kubernetes Pod exec。
- 节点 `registries.yaml` 的 SSH 下发由路由注入基础设施适配器，delivery Handler 不再依赖 API 根包的全局状态。

迁移：

```text
internal/api/delivery/registry_proxy_handler.go
internal/api/delivery/node_registry_mirror_handler.go
internal/service/registry/proxy_service.go
internal/service/registry/mirror_service.go
```

两个 Service 可以共享 Registry 校验能力，但保持独立的业务入口，避免制品库、代理和节点镜像源互相耦合。

### 阶段 4：平台发布和应用发布

状态：已完成（2026-08-29）。

已落地内容：

- 平台发布 HTTP Adapter 已迁移至 `internal/api/delivery/platform_handler.go`；平台镜像前缀校验、Webhook 签名和防重放、发布记录、自更新 Deployment reconcile 与未完成发布恢复收敛至 `internal/service/platform/release_service.go`。
- 应用发布在复用既有 `internal/application.Service` 状态机的基础上新增 `ReleaseWorkflow`。模板发布、当前成功版本重启、重试和回滚统一在该工作流中准备运行期镜像仓库凭据、节点镜像源校验地址、发布快照和模板关联信息。
- 发布后的应用入口同步，以及 ConfigMap/Secret 受管键登记已下沉至 `ReleaseWorkflow`；HTTP Handler 不再编排异步发布执行、镜像验证配置或受管文件持久化。
- `ReleaseWorkflow` 的测试覆盖模板 Secret 与仓库凭据不进入发布快照、节点已下发镜像源选择；既有 API 集成测试覆盖重启、入口同步和受管文件工作流。
- REST 路径、请求字段、响应字段、异步执行模型及原有中文错误提示保持不变。

先迁移平台发布：

```text
internal/api/delivery/platform_handler.go
internal/service/platform/release_service.go
```

再整理应用发布，优先复用已有的 `internal/application.Service`，逐步下沉镜像校验、发布快照、重启和回滚逻辑。

### 阶段 5：基础设施模块

实施状态：已完成（2026-08-30）。服务器、节点、PVC、域名、证书、Ingress、网络诊断、SSH 终端、Pod 终端、节点加入进度、通用 Kubernetes 资源和 CoreDNS HTTP 边界均已迁入 `internal/api/infrastructure`。服务器/节点生命周期由 `cluster.Service` 提供，存储与网络规则分别由 `storage.Service`、`network.Service` 提供。统一 SSH 执行、参数构造、连通性/前置检查、主机名清理和资源统计解析已收敛到 infrastructure 共享适配器，Agent、维护、镜像源、PVC 与集群适配器共同复用。根包旧 Handler、注入适配和重复测试已删除，生产路由直接绑定 infrastructure Handler；CoreDNS 状态格式化辅助统一由 `internal/service/network` 提供，Agent 与 Infrastructure Handler 共同复用。

按领域迁移：

```text
internal/api/infrastructure/
  server_handler.go
  node_handler.go
  node_join_progress_handler.go
  server_network_diagnostics.go
  server_terminal_handler.go
  pod_terminal_handler.go
  k8s_handler.go
  cluster_dns_handler.go
  domain_handler.go
  cert_handler.go
  ingress_handler.go
  ssh.go
  storage_handler.go
  network_handler.go
```

对应 Service：

```text
internal/service/cluster/
internal/service/storage/
internal/service/network/
```

### 阶段 6：Agent 和系统模块

实施状态：已完成（2026-08-30）。Agent 运行时 API、操作审批、维护与制品下载，以及 Tailscale、系统组件、监控、日志、告警和审计 HTTP 边界均已迁入独立包。Router 与平台启动入口直接绑定新包；Kubernetes 客户端由 Router 显式注入 system 包，Agent 的 SSH、Registry、维护和监控辅助逻辑在 Agent 包内复用 infrastructure 适配器。原根包 Handler、测试和重复绑定已删除，REST 路径、鉴权和响应契约保持不变。

迁移后的领域包：

```text
internal/api/agent/
internal/api/system/
```

包括 Agent 操作审批、Tailscale 诊断、系统组件、审计和监控。上述模块已在前置 Service 边界稳定后完成物理迁移。

## 单模块迁移流程

每个模块必须按以下顺序执行：

1. 记录迁移前测试结果。
2. 梳理 Handler 对外路由、请求结构、响应结构和依赖。
3. 先为 Service 定义测试和必要接口。
4. 将业务逻辑从 Handler 下沉到 Service。
5. 将持久化访问下沉到 Repository 或 Store 适配层。
6. 将 Kubernetes 操作收敛到 K8s Adapter。
7. 移动 Handler 和对应测试到领域 API 包。
8. 保持原路由、鉴权、错误码和响应格式不变。
9. 完成格式化和验证。
10. 本地验证相关页面和完整工作流。
11. 独立提交一个模块迁移 commit。

## 验证门禁

每个模块完成后必须执行：

```bash
gofmt -w <changed-go-files>
go test ./...
go build ./...
cd web && npm run build
```

涉及前端调用的模块还需要验证：

- API 请求仍然命中后端，而不是 SPA fallback。
- 浏览器错误以弹窗或页面状态展示。
- 原有鉴权和 token 刷新流程不变。
- 成功、失败、加载和空状态均可操作。

在全量验证通过前，不进行下一模块迁移。

## 提交约定

每个模块独立提交，提交信息明确说明迁移范围，例如：

```text
refactor(registry): extract managed registry service layer
```

不要在同一个提交中混入无关 UI 重构、数据库迁移或格式化全仓库文件。

## 首个执行任务

从阶段 0 开始，完成基线记录后，针对 `managed OCI Registry` 输出依赖清单和 Service 接口设计，再进入代码迁移。首个模块确认通过后，才继续处理 Registry Proxy 和节点镜像源。
