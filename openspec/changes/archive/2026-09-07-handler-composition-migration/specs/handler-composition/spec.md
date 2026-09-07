# handler-composition Specification

## ADDED Requirements

### Requirement: Bootstrap owns production Handler construction

系统 SHALL 仅在 `internal/bootstrap` 的生产组装路径创建 API Handler、领域 Service 和 Kubernetes Adapter。

#### Scenario: Registering application routes

- **WHEN** 平台启动并注册 Gin 路由
- **THEN** `api.RegisterRoutes` SHALL 只绑定由 Bootstrap 提供的 Handler，且不得创建 Service、Adapter 或 Handler

### Requirement: Handlers use explicit narrow dependencies

系统 SHALL 让 Handler 仅依赖 Repository 接口、领域 Service、窄 Adapter 接口和显式配置。

#### Scenario: Handling a Kubernetes-backed request

- **WHEN** 一个 Handler 处理 Kubernetes 相关请求
- **THEN** 它 SHALL 调用注入的窄 Adapter 或 Service，而不得读取完整 Kubernetes Client 或创建替代 Service

### Requirement: Existing HTTP contracts remain stable

系统 SHALL 保持既有 REST 路径、方法、JWT 鉴权、中间件顺序、响应包装和错误语义。

#### Scenario: Calling an existing endpoint after composition migration

- **WHEN** 已授权客户端使用既有请求调用任一已迁移端点
- **THEN** 系统 SHALL 返回与迁移前兼容的状态码、响应字段和业务结果

### Requirement: Handler dependencies are testable in isolation

系统 SHALL 支持使用 Fake Repository、Service 和 Adapter 创建 Handler 测试，而不依赖真实 Store 或 Kubernetes Client。

#### Scenario: Propagating request context through a Handler

- **WHEN** Handler 调用注入的远程 Adapter
- **THEN** Adapter SHALL 接收到 HTTP 请求的 Context，且超时或取消错误 SHALL 映射为既有 API 错误语义
