# Agent Platform Capabilities

## Why

Nanobot Runtime 已能在 Cylism 平台中聊天，但无法在不暴露 Kubernetes 凭据、用户 JWT 或通用 Shell 的前提下诊断和处置平台问题。运维 Agent 需要受限的平台工具、可撤销的 Runtime 身份、最小授权范围、人工审批和完整审计，才能将模型能力安全地接入既有平台操作能力。

## What Changes

- 在 Manager Go 模块内新增 `cmd/cylism-cli`，以固定子命令和 JSON 协议请求 Manager 的 Agent API。Manager 镜像构建该二进制并通过认证的内部制品端点向受管 Runtime 分发；不提供任意 HTTP、`kubectl` 或 Shell 透传。
- Manager 新增 Agent 控制面：Runtime 工作负载身份认证、能力授权与范围校验、只读查询、变更操作审批、异步执行和不可变调用审计。
- Runtime 为 Nanobot 增加受控的 Cylism 工具适配层、安装器和仅面向 Manager 的短期投影 ServiceAccount Token。平台请求安装时，安装器将经验证的 CLI 写入非持久 `emptyDir`；只有平台启用工具、挂载身份并授予能力后才可调用；内置动作目录仅提供可配置选项，不构成默认授权；继续禁用 Nanobot 通用 `exec`。
- Runtime 详情页增加 Agent 能力配置和审批队列，支持授权、撤销、范围配置与审批执行。
- 首期开放受范围限制的诊断能力，以及扩缩容、发布重试和回滚等必须人工审批的变更能力。

## Capabilities

### New Capabilities

- `agent-platform-capabilities`: 为受管 Runtime 提供基于工作负载身份、Capability 和 Scope 的平台运维能力，并将写操作纳入审批与审计闭环。
- `agent-platform-cli`: 提供仅含显式命令 schema 的 Runtime 内部 CLI，作为 Nanobot 受控工具与 Manager Agent API 的客户端。

### Modified Capabilities

- `agent-runtime`: 受管 Nanobot Runtime 保持禁用通用 Shell，但可挂载无 Kubernetes RBAC 的短期身份，并运行固定的 Cylism 工具适配层。
- `dashboard-audit`: 审计记录 Agent 调用、拒绝、审批和执行结果，并保留 Runtime 与聊天会话关联。

## Non-Goals

- 不开放 `tools.exec`、任意 Shell、Terminal、任意 Manager HTTP 转发、`kubectl` 或 Kubernetes 凭据。
- 不允许从公网、用户可写 PVC 或任意 URL 下载 CLI，也不允许 Manager 通过 Kubernetes `exec`/`cp` 修改运行中的 Pod。CLI 只可由受管 init container 从 Manager 内部制品端点安装至 `emptyDir`，并校验平台声明的版本和 checksum。
- 不向 Runtime 返回 Secret 值，不在首期返回未脱敏的 ConfigMap 内容。
- 不在首期开放删除资源、节点驱逐/删除、节点标签修改、Service/Ingress/证书修改或任意镜像更新。
- 不由模型判断或自动批准写操作；上游 Nanobot 的 Hook 或 UI 确认不作为授权边界。
- 不在审批完成后向已结束的 Nanobot 流式响应自动注入新消息；用户可在后续对话查询操作状态。

## Impact

- `cmd/cylism-cli`、Manager Dockerfile、`internal/model`、`internal/store`、`internal/api`、`internal/runtime`、`internal/k8s` 增加 CLI 制品、安装状态、Agent 身份、授权、审批、审计和服务层能力。
- `web/src/views`、Runtime 详情页和 API 客户端增加 Agent 授权与审批 UI。
- `cylism-nanobot-runtime` 增加固定工具适配器、受控安装器和投影工作负载身份挂载；需要先完成 Nanobot 0.3.0 自定义工具扩展的 PoC。
