## Why

应用发布模板当前固定渲染 TCP `ClusterIP` Service，公网端点固定渲染 HTTP Ingress，且 ConfigMap/Secret 只能注入环境变量。该模型不能表达 UDP 或其他四层服务，也不能将证书、配置等 Kubernetes 对象以文件形式提供给容器。

需要在不改变既有 HTTP 发布流程的前提下，补齐通用的 L4 Service 和只读文件投影能力。

## What Changes

- 扩展模板 Service 定义，支持多个独立协议端口，以及 TCP/UDP、ClusterIP/NodePort/LoadBalancer、NodePort 与 externalTrafficPolicy。
- 允许公网端点显式选择 HTTP Ingress 或 Service 直出；Service 直出不创建 Ingress。
- 允许模板把已存在的 ConfigMap 或 Secret 的指定 key 只读挂载到容器文件路径。
- 更新模板编辑器，按所选网络模式显示对应字段，并保留环境变量配置入口。

## Capabilities

### Modified Capabilities

- `application-release`: 模板可表达四层 Service、非 HTTP 公网暴露与只读配置文件投影。

## Non-goals

- 不实现任意 UDP/QUIC 的 HTTP Ingress 转换或协议代理。
- 不实现通用负载均衡器供应商安装、云安全组或防火墙自动配置。
- 不允许模板挂载任意主机路径、可写 Secret，或读取其他命名空间资源。
- 不改变未设置新字段的历史模板行为。

## Impact

- 后端 ReleaseSpec、资源渲染、预检与就绪判断将扩展 Service 与投影资源的语义。
- 前端模板编辑器和应用入口展示将按网络模式呈现 Service 端点，而非始终构造 HTTP URL。
- 现有应用模板、发布快照和 HTTP Ingress 流程保持兼容。
