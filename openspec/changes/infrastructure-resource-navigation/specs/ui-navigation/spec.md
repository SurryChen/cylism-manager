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

### Requirement: 存储卷以集群资源方式查看

系统 SHALL 默认展示集群内所有 PersistentVolumeClaim，且不要求用户先选择项目、环境或应用。

#### Scenario: 用户进入存储卷页面

- **WHEN** 用户打开 `/storage`
- **THEN** 系统展示所有命名空间中的 PVC
- **AND** 每个 PVC 展示命名空间、托管来源和工作负载引用

#### Scenario: 用户按应用归属筛选存储卷

- **WHEN** 用户选择项目或环境筛选条件
- **THEN** 系统仅展示归属于该项目或环境的托管 PVC
- **AND** 用户清除筛选后重新看到集群全部 PVC

### Requirement: 服务器资源监控视图

系统 SHALL 在服务器页面提供与基本配置并列的资源监控视图。

#### Scenario: 用户查看服务器资源概览

- **WHEN** 用户切换到服务器资源监控视图
- **THEN** 系统通过一个批量请求展示每台登记服务器的 CPU、内存、根磁盘、负载和采样状态
- **AND** 不可达服务器展示明确错误状态而非零值指标

#### Scenario: 资源监控视图自动刷新

- **WHEN** 资源监控视图处于激活状态且浏览器页面可见
- **THEN** 系统每十秒刷新一次资源数据
- **AND** 上一次采集未结束时不创建并发刷新请求

### Requirement: 集群监控提供按风险下钻的视图

系统 SHALL 将集群监控拆分为概览、节点、工作负载和诊断视图，且默认展示概览。

#### Scenario: 用户打开集群监控

- **WHEN** 用户访问监控页面且 VictoriaMetrics 已就绪
- **THEN** 系统默认展示监控状态、可用节点及 CPU、内存、根磁盘的资源峰值
- **AND** 系统展示按节点分组的历史资源趋势
- **AND** 原始 PromQL 查询不占用默认视图

#### Scenario: 用户切换到节点监控

- **WHEN** 用户选择节点视图并选择一个节点
- **THEN** 系统展示该节点的当前 CPU、内存、根磁盘与网络使用情况
- **AND** 系统展示所选时间范围内的节点趋势

### Requirement: 监控历史查询受范围限制

系统 SHALL 仅允许监控图表请求预定义的历史时间范围。

#### Scenario: 用户请求趋势图数据

- **WHEN** 前端请求 1h、6h、24h 或 7d 的历史指标
- **THEN** 服务端计算 VictoriaMetrics 查询的起止时间与采样间隔
- **AND** 服务端返回对应的时间序列结果

#### Scenario: 用户请求未支持的时间范围

- **WHEN** 前端请求预定义范围以外的历史指标
- **THEN** 系统拒绝该请求并说明支持的时间范围
