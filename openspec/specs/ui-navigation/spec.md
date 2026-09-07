# ui-navigation Specification

## Purpose
定义 Glass UI 导航结构，包括桌面端顶部一级导航 + 侧栏二级导航、移动端抽屉导航及路由入口。
## Requirements
### Requirement: 数据管理导航入口
系统 SHALL 将“数据管理”降级为内部维护入口，而不是基础设施主路径中的常规导航项。

#### Scenario: 普通导航不展示数据管理
- **WHEN** 用户登录后查看顶部栏、桌面侧栏或移动抽屉
- **THEN** 常规导航中不显示“数据管理”作为基础设施主入口

#### Scenario: 内部入口仍可访问
- **WHEN** 用户通过内部维护链接或直接访问 `/db-admin`
- **THEN** 系统仍允许进入数据管理页面

### Requirement: 运维导航入口调整
系统 SHALL 使用顶部栏承载概览、应用、基础设施和记录三个一级入口，并使用侧栏承载当前一级入口的二级导航；系统 SHALL 不显示已废弃的"导入"入口。

#### Scenario: 桌面顶部栏和侧栏显示分层运维入口
- **WHEN** 用户登录后以桌面视口访问服务器页面
- **THEN** 顶部栏显示概览、应用、基础设施和记录，基础设施具有可见选中状态，侧栏显示服务器、路由、证书和资源，不显示"导入"

#### Scenario: 应用入口显示发布中心
- **WHEN** 用户登录后访问 `/applications`
- **THEN** 顶部栏中应用具有可见选中状态
- **AND** 侧栏显示应用列表入口，点击后保持在应用发布中心

#### Scenario: 移动抽屉显示所有运维入口
- **WHEN** 用户登录后以移动视口打开导航抽屉
- **THEN** 抽屉显示与桌面侧栏一致的运维入口，不显示"导入"

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

### Requirement: 系统设置导航入口
系统 SHALL 提供“系统设置”页面入口，用于管理 Tailscale auth key、SSH 默认超时和控制面相关设置。

#### Scenario: 记录与系统下显示系统设置
- **WHEN** 用户访问任意记录与系统模块页面
- **THEN** 侧栏显示“系统设置”入口，指向 `/settings/system`

### Requirement: Delivery center navigation entry

The system SHALL expose a Delivery Center navigation entry for authenticated users. The entry SHALL include a Registry view and SHALL not replace existing node mirror or application release navigation.

#### Scenario: Open the managed Registry view

- **WHEN** an authenticated user selects Delivery Center and then Registry
- **THEN** the system SHALL navigate to the managed Registry view and display its deployment, endpoint, transport, data-node and node-application status
