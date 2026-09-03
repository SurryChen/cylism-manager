## Why

`internal/api/agent/agent_handler.go` 已包含超过一千行的多类 Agent API 端点。它们共享认证与依赖，但在同一文件中混合工作负载、Registry、观测和维护入口，增加定位与回归测试维护成本。

## What Changes

- 在保持 `package agent` 不变的前提下，按 Agent API 资源族移动 Handler 方法与测试文件。
- 保留 `AgentHandler` 的构造、认证、能力校验、审计、公开方法签名、HTTP 路径与响应契约。
- 为工作负载、Registry、观测、维护端点建立与源文件对应的测试文件。
- 不处理 Agent 对 Infrastructure SSH 实现的横向依赖，也不处理 CoreDNS 辅助逻辑去重。

## Capabilities

### New Capabilities

- `agent-api-organization`: Runtime Agent HTTP API 按资源族组织，同时保持既有外部契约。

### Modified Capabilities

- 无。

## Impact

受影响代码限于 `internal/api/agent` 的源文件和测试文件。REST 路由、Bootstrap 组合、数据库结构、Kubernetes 调用和前端接口均不变。
