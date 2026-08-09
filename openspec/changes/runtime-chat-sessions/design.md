# Design: Runtime Chat And Session Proxy

## Architecture

Manager 作为 Agent 控制平面/BFF：所有聊天、会话读取都经由 Manager 鉴权与代理，浏览器永不接触 Runtime 端口与密钥。

```text
浏览器 ChatDrawer
  │ JWT
  ▼
Manager（鉴权 / SSE 中继 / 会话读取代理）
  │ 统一契约 + Runtime API Key（集群内）
  ▼
Runtime Pod（cylism-nanobot-runtime）
  ├─ api          :8900   nanobot serve（原生 chat，stream=true）
  ├─ gateway      :18790  nanobot gateway（现状，不暴露）
  └─ session-api  :18800  新增只读 sidecar
```

## Unified Contract

Manager 对外（JWT）：

```text
GET  /api/runtimes/:id/chat/sessions
     → { sessions: [{ id, title, updated_at, message_count }] }
GET  /api/runtimes/:id/chat/sessions/:sid/messages
     → { messages: [{ role, content, created_at }] }
POST /api/runtimes/:id/chat          （SSE）
     body: { session_id?, message }
     data: {"type":"delta","content":"..."}
     data: {"type":"done"}
     data: {"type":"error","message":"..."}
```

Runtime 对内（Bearer Runtime Key）：

```text
POST /v1/chat/completions           nanobot 原生
GET  /v1/sessions                   sidecar
GET  /v1/sessions/{id}/messages     sidecar
```

adapter 接口扩展：

```go
type Capability string // "chat" "sessions"
Definition() 增加 Capabilities []Capability
ChatEndpoint(instance)    (string, error)
SessionEndpoint(instance) (string, error)
```

## Session Sidecar（方案 B）

选型：aiohttp（nanobot 的 `api/server.py` 已依赖，零新增）；同镜像第三容器，共享 `/data`；鉴权与 chat 一致（`CYLISM_RUNTIME_API_KEY`）。

实现原则：**不重写会话逻辑**。sidecar 直接复用 nanobot 内部读取能力（如 `nanobot.webui` 的 `list_webui_sessions`），只包一层只读 HTTP。会话存储格式的耦合被隔离在镜像内部，Manager 只见稳定契约。

实施第一项任务：读取 pin 的上游提交 `bd8d3ad5`，确认会话列表/消息读取函数、`api:xxx` session_key 与列表会话的对应关系，再决定 sidecar 是直接 import 还是做少量适配。

## Manager Chat Handler

SSE 中继要点：

- 响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`X-Accel-Buffering: no`。
- 逐条解析上游 `data:` 并转发，`flush` 每个事件。
- 每 15 秒发送 `: ping` 心跳注释，防止中间层超时。
- 客户端断开（`r.Context().Done()`）立即取消上游请求，停止模型生成。
- 错误分两段：响应头发出前返回 JSON 错误；发出后发送 `{"type":"error"}` 事件并关闭。

会话读取为无状态代理：Manager 鉴权后转发 sidecar 响应，不做缓存与落库。v1 不做用户↔会话映射表。

## Kubernetes

- `NanobotAdapter.Workload` 增加第三个容器（端口 18800、健康探针、共享 `/data`、命令为 sidecar 入口）。
- `applyService` 支持多端口：保留 8900 对外，新增 `session-api: 18800` 仅 ClusterIP 内部访问。

## Frontend

- 自研 `ChatDrawer.vue`：右侧抽屉、消息气泡、流式光标、停止按钮、自动滚动、会话列表与历史加载，全部复用现有 Design Tokens。
- `chatStream()` 用 `fetch + ReadableStream` 解析 SSE（EventSource 不支持 POST），配合 `AbortController` 实现停止。
- 打开抽屉时动态拉取会话与历史；关闭重开重新从 Runtime 读取，前端不做历史真相源。

## Risks And Mitigations

| 风险 | 缓解 |
|---|---|
| nanobot 会话存储格式随上游变化 | 读取逻辑只存在于 sidecar，Manager 只见稳定契约 |
| `list_webui_sessions` 耦合 WebUI 上下文 | 实施前先验证；必要时 sidecar 直接读工作区会话文件 |
| SSE 被中间层缓冲 | `X-Accel-Buffering: no` + 心跳 + 不缓冲配置 |
| Service 多端口改动影响现有部署 | 仅在 Nanobot adapter 上新增端口，其他 Runtime 不受影响 |

## Alternatives Considered

- 方案 A（复用 gateway WebUI 的 `/api/sessions`）：依赖未启用的 WebSocket 通道与 WebUI 上下文，鉴权/格式不稳定，放弃。
- Manager 落库会话历史：与 Runtime 记忆双份存储、易失一致，放弃。
- 第三方聊天组件（chat-ui-kit-vue）：自带设计体系，覆盖成本高，与玻璃风格冲突，放弃。
