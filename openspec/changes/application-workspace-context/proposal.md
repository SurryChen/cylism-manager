# Application Workspace Context And Namespace Ownership

## Why

应用模块当前以跨项目、跨环境的资源列表作为默认入口，用户在创建应用、域名或配置时反复手工选择项目、环境和命名空间。`Environment.Namespace` 没有全局唯一约束，多个项目可以绑定同一个 Kubernetes Namespace，破坏受管域名、TLS Secret、配置与 RBAC 的隔离边界。

## What Changes

- 为应用模块引入持久的工作上下文：当前项目与环境，环境派生唯一 Namespace。
- 将应用模块拆分为工作台、项目与环境、镜像仓库授权、全局概览四个二级入口。
- 工作台中的应用、发布、受管域名、运行状态与配置默认按当前环境 Namespace 过滤；创建动作不再手填 Namespace。
- 强制 Environment 与 Namespace 一对一：新建/更新时拒绝复用其他 Environment 已绑定的 Namespace，并验证 Kubernetes Namespace 的项目归属标签。
- 检测存量重复 Namespace 绑定，提供冲突视图与迁移入口；在冲突全部解决前不自动创建数据库唯一索引。
- 受管域名改为关联 `environment_id`，由环境导出 Namespace，避免裸 Namespace 字符串漂移。

## Non-goals

- 不自动迁移 Kubernetes 工作负载、Secret、Certificate 或 DNS 记录到新 Namespace。
- 不允许共享生产 Namespace；本变更不实现共享开发 Namespace 例外。
- 不改变集群级资源，如节点、DNS 凭据、ClusterIssuer、Chart 仓库的管理边界。
- 不引入多租户授权模型；项目级 RBAC 是后续能力。
