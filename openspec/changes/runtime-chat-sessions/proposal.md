# Runtime Chat And Session Proxy

## Why

平台当前只把 Agent Runtime 当作可部署、可健康检查的资源管理，用户无法在平台内与 Agent 对话。nanobot 原生 API 只暴露 `/v1/chat/completions`、`/v1/models`、`/health`，会话数据虽然由 nanobot 持久化在工作区，但没有可用的会话列表和历史读取接口（会话/历史路由属于未启用的 WebUI/网关内部 handler）。

后续目标是运维 Agent、飞书知识库、书签整理等多 Agent 形态，以及会话代理、记忆查询、SSE 流式响应和 cylism-cli。阶段一先打通“平台内聊天 + 流式回复 + 会话历史可续”的最小闭环，并定下统一的 Runtime 聊天契约，为多 Agent 铺路。

## What Changes

- 定义统一的 Runtime 聊天契约：chat（SSE 流式）、sessions 列表、会话消息读取；adapter 增加 `Capabilities`、`ChatEndpoint`、`SessionEndpoint`。
- Manager 新增 `/api/runtimes/:id/chat`、`/chat/sessions`、`/chat/sessions/:sid/messages` 路由：JWT 鉴权、Runtime API Key 只在 Manager 内使用、SSE 透传与取消传播。
- `cylism-nanobot-runtime` 镜像新增 `session-api` 只读 sidecar（aiohttp），复用 nanobot 内部会话读取能力，对外暴露统一契约，共享 `/data` 工作区，使用 `CYLISM_RUNTIME_API_KEY` 鉴权。
- 前端自研玻璃风格 `ChatDrawer`，详情页入口，支持流式逐字渲染、停止生成、会话列表与历史动态加载。
- 会话历史状态归 Runtime 管理，Manager 不落库、不缓存。

## Non-Goals

- 不在阶段一实现多用户会话隔离（假设单用户，一个 Runtime 一个会话视角）。
- 不在 Manager 持久化会话消息，不做消息级审计。
- 不实现记忆查询、工具调用、Agent-Tools API、cylism-cli。
- 不引入第三方聊天 UI 组件库。
- 不公开暴露 nanobot Gateway 或 WebUI。

## Impact

- `cylism-nanobot-runtime`：新增 sidecar 代码、Dockerfile 入口、镜像测试。
- `internal/runtime`：adapter 契约扩展、Workload 增加第三容器、Service 支持多端口。
- `internal/agent`（新增）：契约类型、SSE 读写、Runtime HTTP 客户端。
- `internal/api`：chat handler 与路由注册、鉴权复用。
- `web`：`ChatDrawer.vue`、`api/index.js` 流式请求封装、`RuntimeManagement.vue` 入口。
