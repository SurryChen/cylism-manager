# Application Workspace Context Delta

## ADDED Requirements

### Requirement: Application workspace context

系统 SHALL 以项目和环境组成应用模块工作上下文，并由环境唯一导出 Namespace。

#### Scenario: 进入工作台
- **WHEN** 用户打开应用工作台且存在已保存或 URL 指定的有效项目环境
- **THEN** 系统使用该上下文加载应用、发布、域名和运行摘要，并展示当前 Namespace

#### Scenario: 未选择多个候选环境
- **WHEN** 用户存在多个项目或环境但没有有效已保存上下文
- **THEN** 系统要求用户显式选择工作上下文，不自动选择任意环境

### Requirement: Global Namespace ownership

系统 SHALL 确保一个 Kubernetes Namespace 只绑定一个 Environment。

#### Scenario: 创建重复 Namespace 环境
- **WHEN** 用户为不同 Environment 提交已被其他 Environment 绑定的 Namespace
- **THEN** 系统拒绝请求并返回当前绑定项目与环境

#### Scenario: 绑定不属于当前项目的 Namespace
- **WHEN** 用户以 bind 模式选择标有其他 `cylism.io/project-id` 的 Namespace
- **THEN** 系统拒绝绑定

#### Scenario: 绑定系统 Namespace
- **WHEN** 用户尝试绑定 `default`、`kube-system`、`kube-public` 或 `kube-node-lease`
- **THEN** 系统拒绝绑定

### Requirement: Existing duplicate Namespace migration

系统 SHALL 在存量 Environment 出现重复 Namespace 时提供显式冲突可见性，而不自动选择所有者。

#### Scenario: 启动时发现重复 Namespace
- **WHEN** 数据库中多个 Environment 使用同一 Namespace
- **THEN** 系统将其标记为冲突、阻止新的资源绑定，并在项目与环境中展示需要迁移的 Environment

#### Scenario: 冲突解决后
- **WHEN** 用户将重复 Environment 修改为各自唯一的 Namespace
- **THEN** 系统建立 Namespace 唯一索引，并允许对应环境恢复正常资源创建

### Requirement: Environment-owned managed domains

系统 SHALL 让受管域名关联 Environment，而非仅保存 Namespace 字符串。

#### Scenario: 从工作上下文申请域名
- **WHEN** 用户在当前 Environment 创建受管域名
- **THEN** 系统从该 Environment 写入 Certificate Namespace，并保存 Environment 关联

#### Scenario: 导入既有 Certificate
- **WHEN** 用户从当前 Environment 选择一个尚未被平台记录的单域名 Certificate
- **THEN** 系统创建 `imported` 域名资产，并且不修改、重新签发或删除原 Certificate 和 TLS Secret

#### Scenario: 关联历史域名
- **WHEN** 当前 Environment 的 Namespace 中存在未关联 Environment 的历史域名记录
- **THEN** 用户可以将该记录关联至当前 Environment，系统仅更新 Environment 归属并保留原有域名、Certificate 与生命周期配置

#### Scenario: 拒绝跨环境关联
- **WHEN** 用户尝试关联已归属其他 Environment，或 Namespace 与当前 Environment 不一致的历史域名
- **THEN** 系统拒绝关联，且不修改原有记录

#### Scenario: 发布选择域名
- **WHEN** 用户在工作上下文发布公网 HTTPS 应用
- **THEN** 系统仅允许选择同一 Environment 且 Certificate Ready 的受管域名
