# Application Capability Discovery

## Why

`cylism-hysteria-manager` 已不再部署或持有 Hysteria2 实例。它需要从平台发现当前项目中可由其管理的应用，并展示实际暴露的协议端口、发布状态、副本与节点。当前应用模型没有可扩展的服务能力标识，外部管理台只能依赖应用名称、镜像或端口猜测，既不通用也不可靠。

## What Changes

- 为 Application 增加持久化的通用 `capabilities` 字符串列表，并提供应用创建/更新 API 的受控编辑能力。
- 提供按项目、环境与 capability 过滤的只读应用发现 API。
- 提供单个应用的只读运行态 API，汇总 Service 端口和暴露信息、最新 Release、期望/就绪副本及就绪 Pod 所在节点。
- 所有新 API 沿用现有浏览器 JWT 和项目/环境归属校验；响应绝不返回 Secret、模板加密字段或 Kubernetes Secret 内容。

## Capabilities

### Added Capabilities

- `application-capability-discovery`: 可由其他受权的管理台按通用能力发现项目内应用，并读取经脱敏的运行态摘要。

### Modified Capabilities

- `application-release`: 应用可保存稳定的能力标签，模板、Release 与 Service 语义保持不变。

## Non-goals

- 不实现 Secret 明文读取、项目级 Secret 枚举或浏览器委托令牌。
- 不允许外部调用方修改任意模板、配置或 Release；写操作留待下一阶段设计。
- 不根据镜像名、端口号或协议自动推断 capability。
- 不在此变更中改造 `cylism-hysteria-manager` 的前端/API 客户端。

## Impact

- Application 数据模型、创建/读取 API 和应用详情 UI 将支持通用 capability 元数据。
- 平台新增两个只读 API，运行态依赖现有 Kubernetes Service、Deployment/StatefulSet 与 Pod 查询权限。
- Hysteria2 模板/应用可显式标记 `hysteria2`，但平台不会解释该字符串的业务含义。
