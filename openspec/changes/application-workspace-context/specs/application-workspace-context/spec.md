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

### Requirement: Global image registries and project authorization

系统 SHALL 将镜像仓库作为全局资源，由镜像仓库管理页维护项目授权关系；Environment 不参与镜像仓库授权。

#### Scenario: 创建未授权仓库
- **WHEN** 用户创建尚未授权给任何项目的全局镜像仓库
- **THEN** 系统保存该仓库及其凭据配置，并允许后续从镜像仓库页授予项目使用权限

#### Scenario: 项目查看并选择默认仓库
- **WHEN** 用户在项目与环境中编辑项目
- **THEN** 系统展示项目已授权的镜像仓库，并且仅允许从已授权且启用的仓库中选择默认镜像仓库

#### Scenario: 撤销仓库授权
- **WHEN** 用户从镜像仓库页撤销一个项目的仓库授权，且该仓库是项目默认仓库
- **THEN** 系统清空该项目默认镜像仓库，避免发布使用未授权仓库

#### Scenario: 工作台与发布
- **WHEN** 用户进入项目与环境工作台或发布应用
- **THEN** 工作台展示项目默认仓库和已授权仓库，发布表单默认选择项目默认仓库

### Requirement: Application deployment templates and version releases

系统 SHALL 为每个应用维护多份可管理的上线模板，并将每次发布保存为独立的不可变快照。

#### Scenario: 管理多个上线模板
- **WHEN** 用户进入应用详情页
- **THEN** 系统展示该应用的全部上线模板，并允许创建、编辑、停用、删除未被发布记录引用的模板，以及选择一个启用模板为默认模板

#### Scenario: 首次配置上线模板
- **WHEN** 应用尚未配置上线模板
- **THEN** 系统要求用户先在应用详情页填写镜像路径、运行资源、探针和 Service 配置，创建命名模板后才允许发布

#### Scenario: 按模板与版本发布
- **WHEN** 用户选择该应用的一份启用上线模板并提交合法镜像 Tag
- **THEN** 系统从所选模板读取运行配置，以模板镜像路径和 Tag 解析最终镜像，并在 Release 保存模板 ID、模板修订号和不可变运行快照

### Requirement: Application-owned domain binding

系统 SHALL 将公网域名作为应用独立端点管理，而不是上线模板或单次发布的字段。

#### Scenario: 绑定应用域名
- **WHEN** 用户在应用详情页选择同一 Environment 中已就绪的受管域名并保存路径和 TLS 设置
- **THEN** 系统保存应用端点并立即创建或更新该应用的 Ingress，后续发布沿用该端点

#### Scenario: 解绑应用域名
- **WHEN** 用户在应用详情页解绑域名
- **THEN** 系统删除受管理的应用 Ingress 并清除应用端点记录，后续发布仅保留集群内 Service

#### Scenario: 模板或版本变更
- **WHEN** 用户创建、编辑、选择模板或发布新版本
- **THEN** 系统不得修改应用域名绑定；发布仅使用当前应用端点渲染 Ingress

#### Scenario: 稳定 Deployment 滚动更新
- **WHEN** 应用发布新版本
- **THEN** 系统更新同一 Namespace 中以应用名命名的 Deployment；发布序号仅记录在 Deployment 顶层元数据，不得写入 Pod 模板标签

#### Scenario: 确认新版本就绪
- **WHEN** 系统等待发布完成
- **THEN** 系统仅在当前 Deployment generation 已被控制器观测、更新副本和可用副本均达到期望副本且无不可用副本时，将 Release 标记为成功
