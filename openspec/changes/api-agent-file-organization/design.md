## Context

Agent API 服务于 Runtime Agent，`AgentHandler` 统一持有认证、Kubernetes Adapter、可选观测服务、Registry 验证器和维护检查器。当前多数端点位于 `agent_handler.go`，但端点面向的资源族已清晰分化。

## Goals / Non-Goals

**Goals:**

- 让 Agent HTTP 入口可按资源族定位。
- 让测试文件与被测端点文件对应。
- 保持同一 Go package，避免仅为文件整理引入导出类型和跨 package 依赖。
- 保持所有现有 HTTP、认证、错误映射和 Bootstrap 组合行为。

**Non-Goals:**

- 不创建新的 Go package 或修改路由路径。
- 不下沉 SSH 执行能力，不更换 Kubernetes Adapter 或 Service 依赖。
- 不改变 Agent Operation、Artifact Handler 或后台工作流。
- 不去重 Agent 与 Infrastructure 的 CoreDNS 辅助逻辑。

## Decisions

### 使用同一 `package agent` 的文件拆分

将端点移入 `agent_workload_handler.go`、`agent_registry_handler.go`、`agent_observability_handler.go` 和 `agent_maintenance_handler.go`。`agent_handler.go` 保留 `AgentHandler` 定义、构造函数、认证、能力校验和审计。

选择该方案是因为所有文件继续访问相同私有字段和方法，不改变包接口。替代方案是建立多个子 package，但这会强迫认证、模型和 Adapter 类型跨包导出，收益不足。

### 测试与端点同步移动

将现有 `agent_handler_test.go` 中的测试按相同资源族迁移。共享测试构造保留在 `test_helpers_test.go`；端点行为的测试内容保持不变。

选择同步移动是为了让测试定位与生产文件一致。替代方案是保留一个大型测试文件，但会重新形成与源文件相同的维护问题。

## Risks / Trade-offs

- [移动方法时漏掉私有辅助函数或导入] -> 每组移动后运行 `go test ./internal/api/agent`，最终运行路由快照和完整 API 测试。
- [文件移动意外改变公开符号或路由绑定] -> 不改动方法签名与 `routes_public.go`，并用路由快照验证全部路径。
- [测试移动造成共享 fixture 重复] -> 保留单一 `test_helpers_test.go`，只移动具体测试函数。

## Migration Plan

1. 记录当前路由与 Agent 测试基线。
2. 依次移动工作负载、Registry、观测和维护端点及对应测试。
3. 每一步运行 focused test，完成后运行 `go test ./internal/api/...` 与 `git diff --check`。
4. 若出现行为回归，将方法移回原文件；没有数据迁移或部署步骤。

## Open Questions

- 无。SSH 端口下沉和 CoreDNS 辅助逻辑归属已明确不在本变更范围内。
