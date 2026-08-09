# Tasks

## 1. Runtime Session Sidecar

- [x] 1.1 读取上游提交 `bd8d3ad5` 的 session 存储与 `list_webui_sessions` 实现，确认复用点与 `api:xxx` session_key 对应关系，输出结论。
- [x] 1.2 为 sidecar 只读端点（会话列表、消息读取、鉴权）编写 Python 测试。
- [x] 1.3 实现 aiohttp sidecar 并接入 `CYLISM_RUNTIME_API_KEY` 鉴权。
- [ ] 1.4 更新 Dockerfile 与可执行入口，本地构建镜像并验证 `/v1/sessions` 与 `/v1/sessions/{id}/messages`。（Dockerfile 与入口已完成，镜像构建需 CI/有 docker 环境）

## 2. Manager Contract And Relay

- [x] 2.1 定义 `internal/agent` 契约类型与 Runtime HTTP 客户端，先写失败测试。
- [x] 2.2 扩展 adapter：`Capabilities`、`ChatEndpoint`、`SessionEndpoint`。
- [x] 2.3 实现 chat SSE 中继 handler，覆盖鉴权、透传、取消传播、两段式错误。
- [x] 2.4 实现 sessions/messages 只读代理 handler。
- [x] 2.5 更新 `NanobotAdapter.Workload`（第三容器）与 `applyService`（多端口），补充 Go 测试。

## 3. Frontend Chat Drawer

- [x] 3.1 在 `api/index.js` 实现 `chatStream()`（fetch + ReadableStream + AbortController），先写失败测试。
- [x] 3.2 实现 `ChatDrawer.vue`：会话列表、历史加载、流式渲染、停止、自动滚动，并补 Vue 测试。
- [x] 3.3 在 `RuntimeManagement.vue` 详情页接入聊天入口与玻璃风格样式。

## 4. Verification

- [x] 4.1 每项完成后运行受影响的 Go/Vue 测试。
- [ ] 4.2 端到端验收：发送消息流式回复、停止生效、重开抽屉历史完整、未登录 401、Runtime Key 不出现于浏览器。（需部署新镜像与 Manager 后联调）
- [x] 4.3 执行 `go test ./...`、`go build ./...`、`npm --prefix web test`、`npm --prefix web run build`、`openspec validate runtime-chat-sessions --strict` 和 `git diff --check`。
