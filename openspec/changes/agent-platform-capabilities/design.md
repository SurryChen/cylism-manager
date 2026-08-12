# Design: Agent Platform Capabilities

## Context

现有 Runtime 将 `tools.exec.enable` 固定为 `false`，Pod 以非 root 身份运行、没有自动挂载的 ServiceAccount Token，也没有 Manager 用户 JWT 或 kubeconfig。Manager 的浏览器 API 只接受用户 JWT；现有审计只记录成功的变更调用，不能表达 Runtime 身份、拒绝、审批或精确动作快照。

Nanobot 0.3.0 Python SDK 文档确认了完整 Agent Runtime、工具事件与 `AgentHook` 观测能力，但未提供可作为安全边界的命令审批 API。因此 Manager 必须是授权、审批和实际执行的唯一可信点。

## Architecture

```text
Nanobot 专用 Cylism 工具
  -> Nanobot 专用工具适配器
  -> cylism-cli（由 Manager 安装至 emptyDir，固定 argv / JSON）
  -> Manager Agent API（ClusterIP 内部服务）
  -> 投影 ServiceAccount Token 校验
  -> Capability + Scope + 风险策略
  -> 只读同步结果 / 写操作创建审批
  -> Manager 服务层调用现有 K8s、应用与监控能力

Manager 镜像内的 CLI 制品
  -> Manager 内部制品端点（安装身份认证 + checksum）
  -> Runtime 安装 init container
  -> /opt/cylism/bin emptyDir
```

Agent API 与浏览器 API 分离为 `/api/agent/v1/*`。它不接受用户 JWT，也不复用 Runtime Chat API Key。Manager 在验证工作负载身份后从服务端推导 Runtime ID，永不信任 CLI 请求中的 Runtime ID。

## Decisions

### Manager 构建和受管安装 CLI

`cylism-cli` 源码位于 Manager 模块的 `cmd/cylism-cli`，与 Manager 共用协议 DTO 和测试工具链。Manager Dockerfile 在同一 Go builder 阶段构建 `cylism-manager` 与静态 `cylism-cli`，最终 Manager 镜像在固定只读路径 `/usr/local/lib/cylism/runtime-tools/cylism-cli` 保存 CLI，并在 `/usr/local/lib/cylism/runtime-tools/cylism-cli.manifest.json` 保存 build version/checksum manifest；不存在独立的 CLI 镜像或独立的 CLI 发布仓库。

Manager 提供仅集群内可访问的内部制品端点 `http://cylism-manager.default.svc.cluster.local:8080/internal/runtime-tools/v1/cylism-cli/linux-amd64`，并在其 `/manifest` 子路径返回 manifest。该端点使用仅安装 init container 可见的、受众为 `cylism-manager-tool-installer` 的短期 projected ServiceAccount Token 鉴权，返回当前兼容 CLI 的二进制和 manifest。它不接受用户指定的 URL、版本或文件名。

平台为 Runtime 保存 `AgentToolInstallation` 期望状态：`not_installed`、`installing`、`installed`、`failed`、`uninstalling`，以及 Manager 发布的 CLI build version/checksum、安装时间和错误摘要。首期不提供用户选择历史 CLI 版本；CLI 与 Manager 的 Agent API `v1` 协议保持向后兼容，Manager 升级后由管理员显式触发已安装 Runtime 的 CLI 更新。

安装和卸载均由 Manager 修改 Runtime Deployment 的期望状态，而不是远程进入已有 Pod：

1. 安装时，Manager 写入期望 CLI manifest，增加专用 `emptyDir`、安装 init container、安装身份卷，以及工具适配器的 CLI 挂载与注册；Deployment 滚动更新。
2. init container 从 Manager 内部制品端点下载到临时文件，校验 build version、checksum、大小和 ELF 可执行格式后原子重命名至 `/opt/cylism/bin/cylism-cli`。`/opt/cylism/bin` 为专用 `emptyDir`，它不得写入 `/data` PVC 或容器根文件系统。
3. 工具适配器 readiness 校验预期 CLI 存在且版本匹配；安装失败时不注册工具，并将安装状态报告为失败。
4. 卸载时，Manager 立即撤销能力和工具身份，然后更新 Deployment 以移除工具注册、安装器和 `emptyDir` 挂载。旧 Pod 的文件仅存于 `emptyDir`，在终止后消失；其后续平台调用已被 Manager 拒绝。

安装或卸载会造成该 Runtime 的滚动重建，应在 UI 中显示对流式会话的影响。权限撤销本身不等待重建。

### 受控 CLI，而非通用命令执行

`cylism-cli` 使用 Go 实现、静态编译、仅输出 JSON。命令为不可扩展的白名单，例如：

```text
cylism-cli cluster status --output json
cylism-cli workload get --namespace N --kind deployment --name X --output json
cylism-cli workload logs --namespace N --pod P --container C --tail 200 --output json
cylism-cli deployment scale --namespace N --name X --replicas 3 --output json
cylism-cli approval get --id OPERATION_ID --output json
```

CLI 在本地校验命令 schema、超时、输出上限和请求 ID，但不作授权决定。禁止 `request METHOD URL`、自由文本 command、环境变量回显和凭据作为命令参数。所有响应使用统一 envelope：`status`、`request_id`、`operation_id`、`data`、`summary`、`retryable`。

Nanobot 集成必须使用专用工具适配器，以固定参数调用 CLI，继续保留 `tools.exec.enable=false`。第一项实现任务是基于精确的 `nanobot-ai==0.3.0` 镜像完成自定义工具注册 PoC；若 `nanobot serve` 无法安全注入该工具，则使用 Python SDK 建立 Cylism API 适配层，而不是开启通用 Shell。

### 本地动作策略

Nanobot 原生 `exec` 不参与审批。需要读取 Runtime 工作区或执行受控本地诊断时，适配器只能提交预定义动作，例如 `workspace.list`、`workspace.read` 和 `runtime.status`，不能提交自由文本命令。

平台内置一个不可扩展的本地动作目录，定义每个动作可用的路径、参数、环境和输出上限。所有 Runtime 的初始有效策略均为 `deny`。Manager 管理员为每个 Runtime 显式配置该目录中每个动作的策略：`deny`、`auto` 或 `approval_required`。内置的推荐策略仅作为 UI 中可选模板，只有管理员保存后才生效，且可在之后随时改为 `deny`；任何动作均不会因为二进制存在而获得免审批权限。

`auto` 只允许已被管理员配置的低风险、只读、固定路径和固定输出上限的动作；脚本解释器、Shell、任意网络工具、任意环境变量和命令拼接始终拒绝。需要执行动作时，Manager 返回绑定 Runtime、动作 ID、规范化参数、工作目录、参数 hash 和过期时间的一次性许可；Runtime 内专用执行器验证许可后才运行，并把截断后的结果回传审计。

### 集群信息只能通过平台能力获取

Nanobot 不直接访问 Kubernetes API。所有集群、工作负载、应用、日志和指标信息都通过 `cylism-cli` 调用 `/api/agent/v1/*` 获取，Manager 使用已有 client-go/应用服务层读取并做 Capability、Scope、脱敏和大小限制。只读操作通常不需要人工审批，但仍必须具备对应 Capability 并记录调用。

### 工作负载身份

每个受管 Runtime 使用独立 Kubernetes ServiceAccount，但不创建任何 RoleBinding。Pod 维持 `automountServiceAccountToken=false`。安装 init container 获得受众为 `cylism-manager-tool-installer` 的短期投影 Token；实际工具适配器获得受众为 `cylism-manager-agent` 的独立短期投影 Token。工具关闭时撤销工具注册和 Manager 侧身份授权；已安装的 CLI 文件只存在于 `emptyDir`。

Manager 通过 Kubernetes TokenReview 校验 audience、namespace、ServiceAccount 和 Runtime 标签映射。该 Token 只能代表 Runtime 调用 Manager，不能调用 Kubernetes API。Manager Agent Service 只以 ClusterIP 暴露；Runtime NetworkPolicy 仅允许访问该 Service、DNS 和已配置的模型提供商。

标准 `networking.k8s.io/v1` NetworkPolicy 不具备 FQDN 规则，不能在不把域名模型服务解析为易失 IP allowlist 的前提下精确表达“仅允许配置的模型提供商”。首期 Runtime 保持由受控工具、无 Kubernetes RBAC 和 Agent API 鉴权限制的最小权限边界；启用 Runtime egress NetworkPolicy 必须以部署集群的 FQDN 策略能力（例如 Cilium FQDN policy）实现，并在该能力可用前作为部署前置检查明确提示，不能以允许所有 HTTPS 或阻断模型连接的伪策略替代。

### 授权与数据模型

新增 `AgentCapabilityGrant`：`runtime_id`、`capability_id`、结构化 scope、`approval_policy`、`enabled`、`version`、创建与更新时间、操作者。Scope 支持 namespace、project、application 和明确资源 allowlist；UI 不提供原始 JSON 编辑器。

新增不可变 `AgentOperation`：`runtime_id`、`capability_id`、`request_id`、`chat_session_id`、脱敏参数摘要、参数 hash、目标 `resourceVersion`、风险等级、状态、审批人、过期时间、执行结果和错误摘要。每个请求，包括拒绝和失败，都写入记录；现有 `AuditLog` 同步记录可查询摘要，并扩展 actor/correlation 字段或由 AgentOperation 关联展示。

首期 Capability：

| 类别 | 首期操作 | 策略 |
|---|---|---|
| 诊断 | 集群摘要、应用/发布、工作负载、Pod、事件、Service/Endpoint、指标、限量日志、Secret 元数据 | 允许后同步返回 |
| 处置 | Deployment/StatefulSet 扩缩容、应用发布重试、应用或 Deployment 回滚 | 始终人工审批 |
| 排除 | Secret 值、删除、节点操作、终端、任意配置修改 | 拒绝 |

所有文本型外部数据（日志、事件、注释）均视为不可信输入，做敏感值脱敏、字段白名单和大小限制后才返回 Agent。

### 审批与执行

变更请求按下列过程处理：

1. Manager 验证身份、能力、scope、参数和值域，读取目标并生成影响摘要。
2. Manager 写入 `pending_approval` 操作，绑定精确参数 hash、授权版本、目标 `resourceVersion` 和有限过期时间；CLI 返回 operation ID。
3. 用户在 Manager UI 批准或拒绝。批准时必须重新读取目标；版本变化则将操作标记 stale，需要重新请求与审批。
4. Manager 的后台执行器调用共享服务层执行操作，并更新结果。CLI 可轮询 `approval get`；同一 `request_id` 只能产生一个操作。

审批不等待 Agent 的流式连接，避免长连接占用和会话串扰。审批完成后，操作面板显示结果；用户的后续消息可让 Agent 查询并解释结果。

### Manager 服务边界与 UI

不得将 Agent 请求转发到既有 Gin Handler。将 Handler 中的业务验证和 K8s 调用下沉到共享服务，浏览器 API 与 Agent API 分别做各自认证、输入 DTO 和审计。

Runtime 详情页增加“Agent 能力”面板：能力开关、可选 namespace/application 范围、审批策略、最近调用和撤销。能力配置通过弹窗完成，避免在 Runtime 列表中展开大量权限细节。`cluster.read` 固定为集群范围；工作负载、日志和扩缩容允许选择一个或多个 namespace，也可选择 `*` 表示全部 namespace，`*` 与具体 namespace 互斥。保存时按 capability/scope 替换该 Runtime 的完整授权集合，撤销旧范围立即生效。

Agent API 提供 `GET /api/agent/v1/capabilities/status`，返回每个已注册 capability 的 `enabled`、有效 namespace 范围和 `approval_required`。Nanobot 通过固定 CLI 命令 `cylism-cli capability status --output json` 查询该状态，不读取 Manager 数据库或 Kubernetes 凭据。

审批队列同时在 Runtime 管理页和聊天窗口提供入口。聊天窗口只展示当前 Runtime 的 `pending_approval` 操作，并通过已有的 Manager 用户 JWT 审批或拒绝；Nanobot 不能批准操作。操作仍由 Manager 绑定 Runtime 身份、精确参数和资源版本后执行，聊天会话关联仅在请求显式携带 `X-Chat-Session-ID` 时记录，不能由 UI 推断。

## Risks And Mitigations

| 风险 | 缓解 |
|---|---|
| Prompt injection 诱导模型执行高危操作 | 服务端固定 Capability、Scope 和参数 schema；模型输出不参与授权 |
| Runtime Token 泄露 | 短期 Token、无 K8s RBAC、TokenReview、按 Runtime 授权、即时撤销与 NetworkPolicy |
| 审批后资源已变化 | 绑定 resourceVersion，执行前重读并使过期操作失效 |
| CLI 扩大为通用后门 | 无自由命令或 URL；严格命令表与参数 DTO；保持 Nanobot exec 禁用 |
| 制品下载被替换或截断 | 安装身份仅可访问固定 Manager 内部端点；下载至临时文件后验证 manifest、checksum、大小和格式再原子安装 |
| Manager 升级后 CLI 协议不兼容 | Agent API 保持 `v1` 向后兼容；UI 显示待更新状态；升级 CLI 由管理员显式触发 |
| CLI 安装/卸载中断对话 | 通过 Deployment 滚动更新并在 UI 提示影响；授权撤销先于 Pod 重建生效 |
| 日志或资源内容泄露凭据 | 结果字段白名单、大小限制、敏感键脱敏、禁止 Secret 值读取 |
| 现有 Handler 逻辑被重复实现 | 先抽取并测试共享服务层，两个 API 入口复用服务 |
| 上游 Nanobot 工具注册不稳定 | 固定 0.3.0 版本并以镜像 PoC 验证；适配代码仅保留在 Runtime 镜像内 |

## Alternatives Considered

### Enable Nanobot `exec` and install the CLI

未采用。模型仍可调用其他二进制或 Shell，CLI 不再是安全边界；工作区限制不能替代平台授权。

### Inject kubeconfig or privileged ServiceAccount into Runtime

未采用。会使 Runtime 绕过平台范围、审批和审计，凭据泄露即获得集群权限。

### Reuse `CYLISM_RUNTIME_API_KEY` or Manager JWT

未采用。前者是 Manager 到 Runtime 的聊天凭据，后者代表用户；两者的主体、轮换和权限边界均不适用于 Runtime 到 Manager。

### Let Nanobot Hooks approve writes

未采用。官方 SDK Hook 是观测/后处理入口，且 Runtime 内逻辑不应成为唯一可信授权点。审批必须由 Manager 服务端持久化和执行。
