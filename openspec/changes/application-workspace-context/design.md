# Design: Application Workspace Context And Namespace Ownership

## Workspace Model

应用模块的默认状态是一个显式工作上下文：

```text
Workspace = Project + Environment + Namespace(derived)
```

上下文存储在浏览器 localStorage，并在 URL query 中镜像 `project_id` 与 `environment_id`，以支持刷新、深链和跨页保持。只有一个可用项目/环境时可自动选中；多个候选时保留最近选择，不静默选择任意生产环境。

二级入口：

```text
应用
├── 工作台      唯一的项目/环境选择入口，聚合应用、发布、受管域名、镜像仓库和运行摘要
├── 项目与环境  全局管理项目、环境、Namespace 所有权和默认镜像仓库
└── 全局概览    跨项目应用、失败发布、证书告警和迁移待办
```

工作台选择项目和环境后，域名、应用和发布操作从该上下文推导 Namespace；镜像仓库显示为当前项目可用资源。域名、镜像仓库和发布记录不再作为一级二级导航，而是从工作台进入其下钻详情。项目与环境和全局概览不显示工作上下文，避免将全局数据误解为已过滤数据。

工作台无可用上下文时提供进入项目与环境管理的创建入口，但项目、环境完整 CRUD 和 Namespace 迁移仍集中在全局管理页，避免重复维护两套复杂表单。

## Namespace Ownership

`Environment.Namespace` 代表唯一的 Kubernetes 隔离边界。新建或更新 Environment 时，Store 在事务中检查是否存在其他 Environment 使用同一 Namespace；若存在，返回冲突。Kubernetes Namespace 的 `cylism.io/project-id` 标签必须为空或等于当前项目 ID，系统命名空间不可被绑定。

数据库迁移不能直接给现存 SQLite 数据加唯一索引：存量重复行会导致迁移失败。启动时：

1. 查询重复 Namespace 分组。
2. 若有冲突，保留应用可启动，标记相关 Environment 为 `namespace_conflict`，拒绝创建新的重复绑定与受管域名申请。
3. 管理员在“项目与环境”中查看冲突，选择一个保留原 Namespace；其余环境通过现有编辑流程迁移到新 Namespace。平台只更新环境绑定，不迁移 Kubernetes 资源。
4. 无冲突后创建 `idx_environments_namespace_unique` 唯一索引；后续数据库与服务层双重保证。

## Managed Domain Migration

`ManagedDomain` 新增 `EnvironmentID`。新建域名必须选择工作上下文环境，服务端从 Environment 导出 Namespace 并拒绝不一致输入。已有域名根据 Namespace 查找唯一 Environment 自动回填；没有匹配或匹配多个 Environment 时标记为未归属，不能用于发布，用户必须在域名页重新绑定环境。

Application 发布使用 `EnvironmentID` 验证域名归属，Ingress、Certificate 与 TLS Secret 仍在该环境 Namespace 中运行。

已有 Certificate 可由用户在对应环境显式接管。接管只创建 `ManagedDomain` 记录并标记 `certificate_ownership=imported`，不修改、重新签发或删除原 Certificate 与 TLS Secret；只支持含一个精确 DNS 名称、Issuer 和 TLS Secret 的 Certificate。

## APIs

- `GET /api/projects/environments/namespace-conflicts`：返回重复 Namespace 与关联环境。
- `GET /api/workspace/overview?project_id=&environment_id=`：返回当前环境的应用、失败发布、受管域名和证书摘要。
- 现有项目、环境、域名和镜像仓库 API 保持兼容；列表增加可选 `project_id` / `environment_id` 过滤。

环境创建和更新的 `bind` 模式必须检查数据库绑定与 Namespace 标签。已有 Namespace 缺少项目标签时，可仅在没有其他绑定时由当前项目认领并补标签。

## Risks And Mitigations

- 旧数据重复导致唯一索引创建失败：先检测并提供可见冲突，不做静默迁移。
- URL 与 localStorage 工作上下文不一致：URL 优先，成功解析后同步本地存储。
- 用户在错误工作上下文创建资源：顶部始终显示项目、环境和 Namespace；服务端继续从 Environment ID 校验，不信任前端。
- 旧受管域名无法映射：保持只读、禁止发布，避免错误地把证书绑定到错误环境。

## Alternatives Considered

- 每个应用一个 Namespace：隔离更强，但会产生大量 Secret、Certificate 与 Service 边界，增加日常运维成本，拒绝采用。
- 只在前端隐藏跨环境资源：无法阻止 API 调用和已有数据冲突，拒绝采用。
- 直接选最早 Environment 作为重复 Namespace 所有者：会改变真实资源归属，拒绝采用。
