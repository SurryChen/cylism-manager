## MODIFIED Requirements

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
