## Context

上游 Nanobot v0.3.0 提供两个独立长驻进程：`nanobot gateway` 默认处理 Gateway、WebUI、Cron、Dream、Heartbeat 和渠道；`nanobot serve` 提供 `GET /health`、`GET /v1/models` 与 `POST /v1/chat/completions` 的 OpenAI 兼容 API。`serve` 的 `session_id` 会映射至 Nanobot 自身的持久会话。API 监听 `0.0.0.0` 时要求配置独立 `api.apiKey`。

Nanobot 的状态目录默认位于用户 Home 下的 `.nanobot`，包含配置、会话、记忆、任务、历史和日志。Cylism 的 Runtime PVC 已挂载到 `/data`，因此应将原生配置固定为 `/data/.nanobot/config.json`，workspace 固定为 `/data/workspace`。

## Goals

- 在平台不管理会话或记忆内容的前提下，部署可持久化的完整 Nanobot Runtime。
- 严格区分模型提供商凭据与调用 Agent API 的凭据。
- 让未来 Runtime 能用受限的 Adapter workload 规范加入，而不是复制 Nanobot 的 Kubernetes 分支。
- 默认以最小权限运行，确保后续受控 CLI 工具是唯一的运维执行入口。

## Decisions

### Adapter 返回受控 workload 规范

Adapter 保留类型、模型协议校验与配置生成职责，并新增由平台数据生成的 workload 规范。该规范只能描述固定的 init/container、端口、探针和安全上下文，用户填写的 Runtime JSON 不参与 Kubernetes 对象拼接。

KubernetesManager 保留 PVC、ConfigMap、Secret、Service、Deployment 的统一创建/更新/所有权校验；根据 Adapter workload 规范写入 PodSpec。这样新增 Runtime 只需注册 Adapter，不会使 HTTP handler 或 Kubernetes manager 增加按类型分支。

### 以一个 Pod 的三个执行阶段运行 Nanobot

1. Init container 从只读 `/etc/cylism/runtime.json` 渲染原生配置至 `/data/.nanobot/config.json`。配置只写 `${CYLISM_MODEL_API_KEY}`、`${CYLISM_RUNTIME_API_KEY}` 引用，明文凭据不写入 PVC。
2. `gateway` 容器执行 `nanobot gateway --foreground --config /data/.nanobot/config.json`，仅监听 Pod 内端口 18790。
3. `api` 容器执行 `nanobot serve --host 0.0.0.0 --port 8900 --config /data/.nanobot/config.json`；Service 和 readiness/liveness probe 只指向此容器的 `/health`。

共享 PVC 让 API 和 Gateway 看到同一份 Nanobot 会话、记忆和任务状态。Service 不暴露 Gateway，避免 WebUI 与渠道管理入口被平台意外暴露。

### 平台生成 Nanobot 原生配置

`responses` 映射为 `providers.openai`，配置 `apiType: "responses"`、`apiBase`、模型名及 `${CYLISM_MODEL_API_KEY}`。`anthropic` 映射为 `providers.anthropic`，配置模型名、`apiBase` 和同一模型密钥引用；其 URL 由上游 SDK 负责处理版本路径。

公共配置固定：`agents.defaults.workspace=/data/workspace`、`api.host=0.0.0.0`、`api.port=8900`、`api.apiKey=${CYLISM_RUNTIME_API_KEY}`、`tools.exec.enable=false`、`tools.restrictToWorkspace=true`、`tools.webuiAllowRemotePackageInstall=false`。用户配置只允许在 Adapter 定义的安全白名单内扩展，第一期不支持覆盖这些字段。

### 两类凭据与部署安全

保留现有模型 Secret 键 `api-key`，新增随机生成的 Runtime API Secret 键。模型凭据仅注入两个 Nanobot 容器为 `CYLISM_MODEL_API_KEY`，API 凭据仅注入为 `CYLISM_RUNTIME_API_KEY`。平台存储 Runtime API 密钥时采用与模型密钥相同的加密机制，列表与状态 API 一律不返回明文。

Pod 使用 UID/GID 1000、`fsGroup: 1000`、`automountServiceAccountToken: false`、非 privileged 容器、只读 root filesystem（挂载必要的临时目录），并删除全部 Linux capabilities。所有容器保留 PVC 的 `/data` 读写权限；仅 init 容器可写配置路径。

### 镜像构建

`cylism-nanobot-runtime` 使用固定 Nanobot 上游提交 `bd8d3ad5b6db273e582fb0864927716f5f8a20e2`（v0.3.0）构建，安装 `nanobot-ai[api]` 所需依赖，创建 UID/GID 1000 的 `nanobot` 用户，并包含用于 init 配置渲染的受限入口命令。镜像不内置模型或 Agent API 密钥。

## Alternatives Considered

### 仅运行 `nanobot serve`

实现简单，但不会运行 Gateway 的 Cron、Dream、Heartbeat、WebUI 和渠道生命周期，不能满足采用 Nanobot 原生能力的目标，因此拒绝。

### 使用单容器 supervisor 同时拉起两个进程

镜像可以实现，但会模糊独立探针、退出状态和资源边界，且平台将自行维护进程管理。因此拒绝，采用 Kubernetes 原生多容器。

### 将平台 ConfigMap 直接当作 Nanobot 配置文件

Nanobot 原生状态需要在自身目录读写，ConfigMap 是只读挂载，且明文密钥不应存入 ConfigMap。故采用 init 渲染至 PVC，并只保存环境变量占位符。

### 复用模型 API 密钥作为 Agent API 密钥

会使平台调用方持有模型提供商权限，无法独立轮换和撤销。因此拒绝。

## Risks And Mitigations

- Gateway 与 API 同时写状态导致冲突：复用上游本地状态机制，先用单副本 Deployment；扩缩容暂不支持。
- PVC 旧 owner 为 root 导致 UID 1000 无法写入：Pod 使用 `fsGroup: 1000`，部署前在测试环境验证 local-path 行为；必要时增加受限权限修复 init 容器。
- API 容器就绪但 Gateway 失败：Gateway 配置独立 readiness/liveness；Deployment readiness 仅在两个容器均 ready 时为 true。
- 运行时配置字段演进：ConfigMap 保留平台内部版本字段，Adapter 对未知或受保护覆盖字段拒绝部署。
- 上游行为变化：镜像按提交固定并使用构建、`/health`、`/v1/models`、受保护 API 四项 smoke test 验证。

## Migration Plan

1. 发布平台 Adapter/workload 支持和 Nanobot 镜像。
2. 用户新建 Nanobot Runtime 时生成独立 API Secret、PVC、ConfigMap、Service 和双容器 Deployment。
3. 已有尚未部署的泛化 Nanobot Runtime 在下一次“部署或更新”时转换为新 workload；原 PVC 保留，配置从平台 ConfigMap 重新渲染。
4. 若新 Pod 不就绪，保留 PVC、ConfigMap 和 Secret 供诊断，平台状态标记失败且不删除已有数据。
5. 回滚时卸载新 Deployment/Service 但默认保留 PVC；重新部署新版本可恢复 Nanobot 状态。
