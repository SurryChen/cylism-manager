# application-release Specification

## Purpose
TBD - created by archiving change application-workload-kinds. Update Purpose after archive.
## Requirements
### Requirement: Applications select a workload controller
The platform SHALL render releases as a Deployment or StatefulSet according to the application workload kind.

#### Scenario: Stateful application release
- **WHEN** an application with workload kind `statefulset` is released
- **THEN** the platform creates or updates a StatefulSet with the release Pod template and stable Service selector

### Requirement: Existing workload type changes protect PVC data
The platform SHALL scale down the prior managed controller before creating the replacement controller for a type migration.

#### Scenario: Deployment with an RWO PVC changes to StatefulSet
- **WHEN** the application has a successful single-replica release and changes workload kind
- **THEN** the old Pod is stopped before the StatefulSet reuses the same PVC

### Requirement: 项目、环境与应用建模
系统 SHALL 支持创建 Project、Environment 和 Application；每个 Application 必须属于一个 Project 和 Environment，Environment 必须绑定一个 Namespace。

#### Scenario: 创建应用草稿
- **WHEN** 已认证用户在 Project 的 `production` Environment 中提交名称为 `order-api` 的 Application
- **THEN** 系统创建 Application 草稿并关联该 Environment 的 Namespace
- **AND** 不创建任何 Kubernetes 资源

### Requirement: 发布定义与预检
系统 SHALL 在创建 Release 前校验无状态 HTTP 服务的镜像、端口、资源限制、Service selector、配置引用、域名冲突和入口依赖；阻断错误不得创建 Release 执行任务。

#### Scenario: Service selector 不匹配
- **WHEN** 用户提交的 Service selector 无法匹配平台将写入 Deployment 的固定标签
- **THEN** 预检返回阻断错误和标签差异
- **AND** 系统不创建 Kubernetes 资源

#### Scenario: HTTPS 依赖不可用
- **WHEN** 用户选择 public HTTPS 入口但 Traefik、cert-manager 或指定 Issuer 未就绪
- **THEN** 预检返回阻断错误并标识不可用依赖

### Requirement: 受控 Kubernetes 资源编排
系统 SHALL 对通过预检的 Release 按 ConfigMap/Secret、Deployment、Service、Certificate、Ingress 或 IngressRoute 的顺序创建或更新资源；默认 Service 类型必须为 ClusterIP。

#### Scenario: 发布公开 HTTPS 服务
- **WHEN** 用户发布镜像、Service 端口、域名和 TLS Issuer 均有效的无状态 HTTP 服务
- **THEN** 系统按依赖顺序创建或更新带 Cylism 管理标签的资源
- **AND** Certificate Ready 后才创建引用其 TLS Secret 的 HTTPS 路由

#### Scenario: 同名非托管资源冲突
- **WHEN** 目标 Namespace 内存在同名但不带 `app.kubernetes.io/managed-by=cylism-manager` 标签的资源
- **THEN** 系统阻断发布并返回资源类型、名称和 Namespace
- **AND** 不修改该资源

### Requirement: 发布状态与步骤进度
系统 SHALL 为每次发布创建不可变 Release 和逐步骤 ReleaseOperation，状态至少包括 `draft`、`validating`、`applying`、`waiting_ready`、`verifying`、`succeeded`、`failed`。

#### Scenario: Pod 未就绪
- **WHEN** Deployment 在 Release 就绪超时前未达到期望 Ready 副本数
- **THEN** Release 状态变为 `failed`
- **AND** 对应步骤记录 Pod 状态、超时原因和时间戳
- **AND** 已创建资源保留供用户诊断或重试

### Requirement: 发布重试与回滚
系统 SHALL 支持重试失败 Release，以及回滚到同一 Application 的上一成功 Release；回滚必须使用完整 Release 快照，而不是只回滚 Deployment revision。

#### Scenario: 回滚到上一成功版本
- **WHEN** 用户对当前成功 Application 选择回滚且上一成功 Release 所引用的配置均存在
- **THEN** 系统创建新的回滚 Release 并重新应用上一成功 Release 的镜像、规模、Service 和路由定义
- **AND** 系统记录回滚来源 Release

#### Scenario: 回滚引用的 Secret 缺失
- **WHEN** 上一成功 Release 引用的 Secret 不存在
- **THEN** 系统阻断回滚并列出缺失 Secret 名称
- **AND** 不修改当前运行资源

### Requirement: 应用发布页面
前端 SHALL 提供应用列表、应用详情和多步骤发布向导，向导必须展示预检结果、渲染后的资源摘要和实时发布步骤。

#### Scenario: 查看正在发布的应用
- **WHEN** 用户进入处于 `waiting_ready` 的 Release 详情
- **THEN** 页面展示每个步骤的状态和最近错误详情
- **AND** 页面不要求用户跳转到多个基础设施资源页才能获知发布进度

