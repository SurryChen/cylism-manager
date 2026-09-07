# bootstrap-transition-cleanup Specification

## Purpose
TBD - created by archiving change bootstrap-transition-cleanup. Update Purpose after archive.
## Requirements
### Requirement: Handler construction has explicit dependencies

系统 SHALL 要求生产和测试 Handler 构造函数显式接收其所需的 Service、Adapter、
Repository 与配置，且 SHALL NOT 在 Handler 构造过程中创建 fallback Service、
Kubernetes Adapter 或默认 Registry。

#### Scenario: Constructing an API Handler

- **WHEN** Bootstrap 或测试创建已迁移的 API Handler
- **THEN** 调用方 SHALL 提供该 Handler 所需的窄依赖，且构造函数 SHALL 直接
  保留这些实例

### Requirement: Bootstrap remains the production composition root

系统 SHALL 仅由 `bootstrap.Container` 在生产路径创建业务 Service 和 Kubernetes
Adapter。

#### Scenario: Searching production construction paths

- **WHEN** 检查 `cmd/platform` 与 `internal/api` 的非测试生产代码
- **THEN** 已删除的过渡构造函数和其 fallback 创建逻辑 SHALL 不存在，且 API
  Router SHALL 只绑定 Bootstrap 提供的依赖

### Requirement: Compatibility cleanup preserves runtime contracts

系统 SHALL 保持既有 HTTP 路径、响应语义、数据库兼容和历史 Registry Proxy
资源迁移行为。

#### Scenario: Handling a legacy Registry Proxy resource

- **WHEN** 客户端请求迁移旧 Docker Hub Registry Proxy 资源
- **THEN** 系统 SHALL 继续执行现有资源重建和状态持久化流程，与构造函数清理
  无关

