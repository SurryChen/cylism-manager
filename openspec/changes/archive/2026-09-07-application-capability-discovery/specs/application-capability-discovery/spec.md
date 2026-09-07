# Application Capability Discovery Delta

## ADDED Requirements

### Requirement: Application capability metadata

系统 SHALL 为每个 Application 持久化一组通用 capability 标识，并将其作为稳定的应用级元数据。

#### Scenario: 保存有效能力标签
- **WHEN** 用户为一个已存在应用提交合法、重复且顺序不同的 capability 值
- **THEN** 系统保存去重、规范化、稳定排序后的列表，并在后续应用读取中返回该列表

#### Scenario: 拒绝无效能力标签
- **WHEN** 用户提交为空、包含非法字符、超过长度或超过数量限制的 capability 值
- **THEN** 系统拒绝更新，且不修改该应用已有的 capability 列表

#### Scenario: 兼容历史应用
- **WHEN** 系统读取未设置 capability 字段的历史应用
- **THEN** 系统返回空 capability 列表，且该应用的模板和发布行为不变

### Requirement: Project-scoped capability discovery

系统 SHALL 提供基于项目、可选环境和可选 capability 的只读应用发现接口。

#### Scenario: 按 Hysteria capability 发现应用
- **WHEN** 已认证调用方请求某项目中 `capability=hysteria2` 的应用
- **THEN** 系统只返回该项目与指定环境范围内包含 `hysteria2` 的应用，不根据名称、镜像或端口猜测匹配项

#### Scenario: 拒绝不属于项目的环境
- **WHEN** 调用方传入不属于指定项目的 environment ID
- **THEN** 系统拒绝请求，不返回任何应用信息

#### Scenario: 不带 capability 的发现
- **WHEN** 调用方在有效项目范围内不传 capability
- **THEN** 系统返回该范围所有应用及其 capability 列表

### Requirement: Sanitized application runtime summary

系统 SHALL 为发现结果和单应用运行态接口返回 Service、Release、工作负载与就绪 Pod 的摘要，且不得返回 Secret 内容或模板私有字段。

#### Scenario: 返回 UDP Service 与节点摘要
- **WHEN** 被发现应用有 UDP LoadBalancer Service、成功 Release 和就绪 Pod
- **THEN** 响应返回该 Service 的 UDP 端口、类型、外部地址、Release 摘要、期望/就绪副本及就绪 Pod 节点

#### Scenario: Kubernetes 资源暂时不可用
- **WHEN** Kubernetes 客户端不可用或其中一项工作负载资源尚未创建
- **THEN** 系统仍返回应用的数据库基础信息，并将对应运行态标记为 unavailable 或空状态，而不使整个发现请求失败

#### Scenario: 不泄漏敏感配置
- **WHEN** 调用方请求发现或运行态信息
- **THEN** 响应不得包含 Secret 值、TLS Secret 名称、模板加密字段、环境变量或文件挂载内容
