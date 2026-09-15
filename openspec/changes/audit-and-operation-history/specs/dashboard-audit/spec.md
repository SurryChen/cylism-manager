## MODIFIED Requirements

### Requirement: 审计日志记录
系统 SHALL 为安全敏感或改变平台状态的业务动作记录审计事件。每条事件必须包含稳定动作名、目标资源类型和标识、结果、时间戳、操作者或自动化来源、可读摘要以及经脱敏的结构化元数据。

#### Scenario: 用户创建站点写入语义化审计
- **WHEN** 已认证用户成功创建站点
- **THEN** 系统写入 action=`site.create`、target_type=`site`、target_id=<新 ID>、outcome=`succeeded` 的审计事件
- **AND** 事件包含当前用户、控制台来源、站点可读标识和不含敏感字段的摘要/元数据

#### Scenario: 镜像源验证写入明确语义
- **WHEN** 已认证用户触发节点镜像源验证并完成请求
- **THEN** 系统记录 action=`registry.mirror.verify` 和镜像源目标
- **AND** 系统不得将该事件归类为 `unknown` 或泛化的 `create`

#### Scenario: 关键动作失败被审计
- **WHEN** 用户或自动化触发的部署、回滚、权限变更或镜像源验证失败
- **THEN** 系统写入 outcome=`failed` 的审计事件
- **AND** 事件摘要描述失败阶段但不得包含 Secret、Token、密码或私钥

#### Scenario: 被拒绝的能力访问被审计
- **WHEN** Agent 因未授予能力而被拒绝访问受控资源
- **THEN** 系统写入 outcome=`denied` 的审计事件
- **AND** 事件关联 Agent Runtime、所需 capability 和拒绝原因

#### Scenario: 普通 Agent 读取不污染审计日志
- **WHEN** Agent 成功执行普通 Pod、Event、DNS、Registry 或监控读取
- **THEN** 系统不得为该普通读取创建默认审计事件

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

## ADDED Requirements

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

### Requirement: 高风险操作审计覆盖
系统 SHALL 审计认证与委托、权限授予/撤销、敏感数据访问、资源创建/更新/删除、部署/回滚/扩缩容、镜像源验证、Terminal 会话以及 Agent 操作申请、批准、拒绝、执行和失败。

#### Scenario: Agent 操作生命周期可关联
- **WHEN** Agent 操作被申请、批准并最终执行
- **THEN** 系统为每个重要阶段写入审计事件
- **AND** 这些事件使用同一 operation_id 关联

#### Scenario: Terminal 会话启动被审计
- **WHEN** 已认证用户启动 Pod Terminal 会话
- **THEN** 系统记录 action=`workload.pod.terminal.start`、Pod 目标、用户和成功或失败结果
