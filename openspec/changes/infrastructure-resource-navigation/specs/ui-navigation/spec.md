## ADDED Requirements

### Requirement: 基础设施按管理对象聚合

系统 SHALL 在基础设施侧栏中展示服务器、集群、Kubernetes 资源、网络访问、存储和监控六个入口。

#### Scenario: 用户进入基础设施模块

- **WHEN** 用户打开任一基础设施路由
- **THEN** 系统仅展示六个聚合入口
- **AND** 当前路由所属入口处于选中状态

### Requirement: 聚合页提供可直达的下钻视图

系统 SHALL 允许用户通过页内导航和 `tab` 查询参数切换聚合页中的子视图。

#### Scenario: 用户进入集群配置

- **WHEN** 用户访问 `/cluster?tab=registry-mirrors`
- **THEN** 系统展示集群页面
- **AND** 节点镜像源视图处于选中状态

### Requirement: 旧资源地址切换到聚合地址

系统 SHALL 将旧的细分资源地址重定向到对应的新聚合地址。

#### Scenario: 用户访问旧服务地址

- **WHEN** 用户访问 `/services`
- **THEN** 系统跳转到 `/resources?tab=services`
