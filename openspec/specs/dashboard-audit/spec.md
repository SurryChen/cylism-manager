## Purpose
平台审计日志记录与仪表盘数据统计，覆盖用户操作追踪、资源变更留痕、后台清理策略以及面向运维人员的状态汇总展示能力。
## Requirements
### Requirement: 审计日志记录
系统 SHALL自动将所有变更操作记录到审计日志，包含操作类型、资源类型、资源 ID、详情、时间戳和操作者 ID。

#### Scenario: 审计日志记录站点创建
- **WHEN** 已认证用户通过 API 创建站点
- **THEN** 系统写入审计日志条目，action="create"，resource_type="site"，resource_id=<新 ID>，detail 包含站点域名，user_id=<当前用户 ID>

#### Scenario: 审计日志包含操作者
- **WHEN** 已认证用户执行变更操作
- **THEN** 审计日志条目中 user_id 字段记录当前认证用户的 ID

### Requirement: 操作日志清理任务
系统 SHALL在后台启动定时任务，按配置的保留天数自动清理过期操作日志。

#### Scenario: 启动时注册清理任务
- **WHEN** 系统启动
- **THEN** 系统启动一个后台 goroutine，每小时执行一次操作日志清理

#### Scenario: 清理过期日志
- **WHEN** 清理任务运行且配置 retention_days > 0
- **THEN** 系统删除所有 created_at 早于（当前时间 - retention_days 天）的操作日志记录

### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: 概览页引导与集群总览

系统 SHALL 在概览页展示面向运维用户的服务健康摘要、集群资源摘要、待关注事项、告警概览和最近操作预览。页面 SHALL 使用真实接口状态渲染，不得依赖固定的 Tailnet、control-plane 或静态在线状态文案；当某个数据源不可用时，页面应明确展示不可用状态并保留其他可用区块。

#### Scenario: 展示服务健康摘要

- **WHEN** 用户打开概览页且数据库概览请求成功
- **THEN** 页面展示服务器、站点、证书风险和最近操作等真实统计，并使用明确的时间窗口和预览口径

#### Scenario: 展示集群摘要

- **WHEN** 用户打开概览页且 Kubernetes 客户端可用
- **THEN** 页面展示节点、命名空间、Deployment、Service、Pod 就绪情况和 K3s 版本；Pod 就绪按 `PodReady=True` 统计，节点版本不一致时明确提示多版本

#### Scenario: 数据源部分不可用

- **WHEN** 数据库、Kubernetes 或 Alertmanager 中任一数据源读取失败
- **THEN** 对应区块显示不可用或部分不可用状态，不把失败显示为零值、空数据或健康状态，同时保留其他成功区块

#### Scenario: 高风险事项可进入处理页面

- **WHEN** 存在触发告警、即将到期或已过期证书、未就绪 Deployment 或未就绪 Pod
- **THEN** 页面在待关注事项中展示风险数量和处理入口，用户可以跳转到告警、证书或 Kubernetes 详情页面

### Requirement: 高风险操作审计覆盖
系统 SHALL 审计认证与委托、权限授予/撤销、敏感数据访问、资源创建/更新/删除、部署/回滚/扩缩容、镜像源验证、Terminal 会话以及 Agent 操作申请、批准、拒绝、执行和失败。

#### Scenario: Agent 操作生命周期可关联
- **WHEN** Agent 操作被申请、批准并最终执行
- **THEN** 系统为每个重要阶段写入审计事件
- **AND** 这些事件使用同一 operation_id 关联

#### Scenario: Terminal 会话启动被审计
- **WHEN** 已认证用户启动 Pod Terminal 会话
- **THEN** 系统记录 action=`workload.pod.terminal.start`、Pod 目标、用户和成功或失败结果

### Requirement: 审计日志包含操作者
系统 SHALL 将事件操作者表达为用户、Agent、系统或委托会话来源，并为后台认证用户保留 user_id 兼容字段。

#### Scenario: 用户变更包含操作者与来源
- **WHEN** 已认证用户执行被审计的变更
- **THEN** 事件记录 actor_type=`user`、当前 user_id 和 source=`console` 或 `api`

#### Scenario: 委托会话包含关联标识
- **WHEN** 受保护控制台委托会话执行被审计的变更
- **THEN** 事件记录 source=`delegation` 和 delegation/request 关联标识

#### Scenario: Agent 自动化包含运行时身份
- **WHEN** Agent 执行被审计的申请、批准、拒绝或操作结果
- **THEN** 事件记录 actor_type=`agent`、关联 Runtime ID 和可读 Runtime 名称

### Requirement: 审计日志查询与展示
系统 SHALL 提供默认面向变更和安全事件的审计查询与展示，支持按结果、动作、目标、操作者、来源和关键词筛选。

#### Scenario: 默认审计列表显示可读事件
- **WHEN** 用户打开审计日志页面
- **THEN** 系统按时间倒序展示时间、结果、动作、目标、操作者和摘要
- **AND** 主列表不得以原始 JSON detail 作为主要信息

#### Scenario: 查看审计事件详情
- **WHEN** 用户打开一条审计事件详情
- **THEN** 系统展示结构化元数据、来源、关联请求/操作标识和兼容的原始 detail
- **AND** 已脱敏字段保持不可见

#### Scenario: 历史审计记录保持可查
- **WHEN** 系统升级后查询既有 `audit_logs` 记录
- **THEN** 系统为缺少新增字段的记录提供 legacy 来源、成功结果和基于原字段的可读回退值
