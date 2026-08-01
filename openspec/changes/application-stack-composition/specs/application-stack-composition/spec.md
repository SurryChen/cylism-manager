# Application Stack Composition Delta

## ADDED Requirements

### Requirement: Application is a single-service boundary

系统 SHALL 将 Application 建模为一个可独立发布、扩缩容、绑定入口和查看运行状态的服务；一个 Application 不得通过应用栈定义渲染多个 Deployment。

#### Scenario: 打开栈组件应用
- **WHEN** 用户从应用栈查看任一组件
- **THEN** 系统打开该组件独立的 Application 详情、模板和发布记录

### Requirement: Environment-scoped application stacks

系统 SHALL 将应用栈模板归属到一个 Environment，并将栈组件作为该 Environment 下的独立 Application 管理。

#### Scenario: 发布 Karakeep 栈
- **WHEN** 用户在一个 Environment 使用 Karakeep 预设创建并发布应用栈
- **THEN** 系统创建或复用 `karakeep`、`karakeep-meilisearch` 和 `karakeep-chrome` 三个独立 Application
- **AND** 每个 Application 生成自己的 Deployment、ClusterIP Service 和 Release 记录

#### Scenario: 组件独立版本
- **WHEN** 用户为栈发布提交组件版本映射
- **THEN** 系统按组件版本创建对应 Application Release，且每个 Release 保存各自不可变快照

### Requirement: Stack presets

系统 SHALL 将 Karakeep 作为可实例化的应用栈预设，而不在 Application 详情中保留专用组件模型。

#### Scenario: 实例化 Karakeep 预设
- **WHEN** 用户填写 Karakeep 与 Meilisearch PVC 以及必填 Secret
- **THEN** 系统创建一个普通环境应用栈模板，并以配置、Secret、PVC 挂载和内部 Service DNS 生成三个组件定义

#### Scenario: 域名归入口应用
- **WHEN** 用户需要暴露应用栈入口
- **THEN** 用户在入口组件 Application 详情中独立绑定受管域名
- **AND** 系统不得在应用栈模板或发布请求中创建 Endpoint
