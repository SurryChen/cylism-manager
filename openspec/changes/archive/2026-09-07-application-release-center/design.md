## Context

Cylism Manager 的运行态事实来源已经是 Kubernetes API，但当前用户必须先后在配置、工作负载、服务、路由和证书页操作。现有 `OperationLog` 可记录长流程，SQLite 可保存元数据；缺少的是应用、Release、资源标签和发布编排边界。

第一期面向单集群、单 Namespace 内的无状态 HTTP 服务。K3s 的 Traefik 提供入口，cert-manager 负责 Certificate 生命周期；Tailscale 仅作为私网连通基础，不承担公网 DNS 或 ACME 验证。

## Goals / Non-Goals

**Goals:**

- 将一次发布的期望状态、执行步骤和结果关联到不可变 Release。
- 以固定依赖顺序创建 ConfigMap/Secret、Deployment、Service、Certificate 和路由。
- 通过 Kubernetes labels 将平台数据库对象与资源建立双向可发现关系。
- 为发布、失败、重试和回滚提供可查询的状态和操作日志。

**Non-Goals:**

- 不实现 Git Webhook、源码构建、镜像仓库、Helm/GitOps 同步或多集群调度。
- 不自动托管 DNS 服务商账号；DNS-01 凭据集成和外部 DNS 自动写入不属于第一期。
- 不支持 StatefulSet、PVC、HPA、蓝绿或金丝雀发布。
- 不修改或接管无 Cylism 管理标签的同名 Kubernetes 资源。

## Decisions

### 1. 数据库存意图与 Kubernetes 运行态分离

新增 `Project`、`Environment`、`Application`、`ApplicationEndpoint`、`Release` 和 `ReleaseOperation` 表。数据库记录发布定义快照、镜像、状态和操作过程；Pod、Deployment、Endpoint、Certificate 条件等实时信息始终从 Kubernetes API 读取。

替代方案是将所有资源详情复制到 SQLite。未采用，因为会造成状态漂移，也会重复 Kubernetes 的控制器职责。

### 2. Release 作为异步、串行的编排任务

`POST /api/applications/:id/releases` 创建 Release 后返回 `release_id`。应用服务在单个 Release 内按以下顺序执行：预检 -> 配置 -> Deployment -> 等待 Pod Ready -> Service -> Certificate -> 等待 Certificate Ready -> 路由 -> 连通性检查。每个阶段写 `ReleaseOperation` 和现有 `OperationLog`，前端轮询读取。

替代方案是让前端依次调用既有资源 API。未采用，因为浏览器关闭会中断流程，无法获得原子化校验、审计与一致的错误语义。

### 3. 只管理带固定标签的资源

平台生成资源时写入 `app.kubernetes.io/managed-by=cylism-manager`、`app.kubernetes.io/part-of`、`app.kubernetes.io/name`、`cylism.io/environment` 和 `cylism.io/release`。创建或更新前检查同名资源：无标签或标签属于其他管理者时阻断；属于当前应用时 apply。

替代方案是用名称推断所有权。未采用，因为名称在 Namespace 中可被手工资源占用，静默接管风险不可接受。

### 4. 默认 ClusterIP，公开入口显式选择

发布定义默认仅创建 ClusterIP Service。用户必须选择 `cluster`、`tailnet` 或 `public` 暴露模式：`public` 才允许域名、HTTPS 和 Ingress；HTTPS 要求 Traefik、cert-manager 和指定 Issuer 均可用。DNS 解析检查属于警告，不能把传播延迟伪造成发布失败。

替代方案是所有服务默认创建公网 Ingress。未采用，因为这扩大攻击面且不适合内部依赖服务。

### 5. 以 Release 快照而非 Deployment revision 回滚

回滚重新应用上一成功 Release 的脱敏发布定义，覆盖镜像、规模、配置引用、Service 和路由。Secret 值不保存到 Release；如旧版本引用的 Secret 已不存在，回滚阻断并说明缺失项。

替代方案是只调用 Kubernetes Deployment rollback。未采用，因为该操作不包含 Service、Ingress、Certificate 和配置关联。

## Risks / Trade-offs

- [发布期间进程重启导致状态中断] -> Release 保留步骤状态；启动时扫描 `applying`/`waiting_ready` Release 并按 Kubernetes 实际状态恢复或标记失败。
- [DNS 或 ACME 延迟] -> Certificate Ready 与公网可达性单独展示；设置超时并提供重试，不删除已创建工作负载。
- [Secret 泄露到快照或日志] -> 发布定义只保存 Secret 名称和键名，输入值只写入 Kubernetes，错误信息必须脱敏。
- [用户手工变更托管资源] -> 详情页展示 drift；V1 下次发布覆盖平台负责字段并记录审计，不自动删除额外字段。
- [单体 Gin 进程内异步任务丢失] -> V1 采用持久化 Release 状态和启动恢复；后续才引入独立队列。

## Migration Plan

1. 部署包含新 SQLite 表和 RBAC 的平台版本，但不自动接管已有资源。
2. 创建应用时只允许新建带管理标签的资源；已有服务可在后续版本通过显式“导入并接管”迁移。
3. 首次发布后从应用详情验证 Pod、Endpoint、Certificate 与路由状态。
4. 回滚时重新应用上一成功 Release；若失败，保留失败 Operation，不删除当前运行资源。
5. 如需禁用该能力，移除应用导航与 API 入口，不影响已有 K8s 资源和基础设施资源页。

## Open Questions

- 公网入口是否统一为单公网节点 Traefik，还是需要支持云负载均衡器。
- DNS-01 的首个支持服务商是否为 Cloudflare，以及凭据是否仅允许 Namespace 级 Secret。
- 生产环境是否需要内置审批，还是由外部 CI 在调用发布 API 前完成审批。
- 镜像仓库允许列表和私有仓库 `imagePullSecret` 的初始管理策略。
