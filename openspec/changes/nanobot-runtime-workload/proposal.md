## Why

现有 Runtime 部署只支持一个通用容器和一份扁平配置，不能按 Nanobot 上游实际形态运行。Nanobot 的 Gateway 与 OpenAI 兼容 Agent API 是独立进程；前者维护原生的会话、记忆、定时任务和渠道能力，后者才是 Cylism 调用 Agent 的入口。继续使用现有模型密钥作为 Agent API 密钥也会扩大凭据暴露面。

## What Changes

- Runtime Adapter 除模型协议声明外，新增受平台控制的 Kubernetes workload 声明能力。
- Nanobot Runtime 部署为共享 PVC 的多容器 Pod：Gateway 运行原生后台能力，API 容器运行 `nanobot serve`；Service 仅公开 API 的 8900 端口。
- 平台将 Runtime 配置转换为 Nanobot 原生 `config.json`，并为 Responses 和 Anthropic 模型协议生成正确的 provider 配置。
- 为 Runtime Agent API 生成并存储独立密钥；模型 API 密钥仅用于 Runtime 到模型提供商的出站请求。
- 默认以非 root 用户运行，关闭 Nanobot Shell 工具、ServiceAccount token 自动挂载和远程 WebUI 包安装。
- 新增 `cylism-nanobot-runtime` 镜像仓库，固定上游 Nanobot 版本并提供 API 依赖、非 root 镜像与启动验证。

## Capabilities

### New Capabilities

- `nanobot-runtime-workload`: 将 Nanobot 的上游 Gateway、持久化状态和 OpenAI 兼容 API 以受控 Kubernetes workload 运行。

### Modified Capabilities

- `agent-runtime`: Runtime Adapter 需要能够声明部署所需的容器、端口、探针和凭据边界，而不再假设所有 Runtime 只有一个容器。

## Non-Goals

- 不在本变更实现 `cylism-cli` 或向 Nanobot 开放任何运维工具。
- 不公开 Gateway/WebUI/聊天渠道到集群外，也不配置第三方聊天渠道。
- 不实现任意命令执行、集群权限透传或任意 Shell。
- 不将平台实现为 Nanobot 会话或记忆的第二份事实来源。
- 不在本变更支持 Nanobot 以外的具体 Runtime 镜像；Adapter 接口会为其预留扩展点。

## Impact

- 修改 `internal/runtime` 的 Adapter、Kubernetes workload 构建及对应 Go 测试。
- Runtime Secret 增加受控的 Agent API 凭据键，部署状态继续由 Kubernetes 资源作为事实来源。
- 新增独立镜像仓库 `/Users/dxm/MyApp/cylism/cylism-nanobot-runtime` 的 Dockerfile、固定依赖和镜像验证。
- 部署运行时将使用现有 `cylism-assistant` 命名空间及其托管 PVC，持久化目录为 `/data`。
