# Stateless External Application Management

## Why

`cylism-hysteria-manager` 已不再承载或运行 Hysteria2。它目前仍保留本地 YAML、进程状态、用户和客户端配置文件的实现，既与新的无状态镜像矛盾，也绕过了 Cylism Manager 对 Kubernetes、Secret、Release 和审计的统一控制。

平台已具备按 capability 发现应用的只读基础能力，但缺少供受权外部管理台安全完成受限配置修改与重新发布的通用契约。直接暴露项目 Secret 或让管理台持有 Kubernetes 凭据都会破坏项目隔离和审计边界。

## What Changes

- 为外部管理台引入短期、签名、项目/环境/capability/action 范围受限的委托凭据。
- 提供通用的“受控结构化文档”资源：绑定某个应用显式拥有的 Secret 或 ConfigMap key，并限制可读摘要和可修改的 YAML/JSON 路径。
- 提供委托调用的受控文档 patch 与应用重启发布接口；平台验证资源归属、并发版本和委托范围，并以原始用户身份记录审计。
- 允许 Hysteria2 将 `config.yaml` 放入应用挂载的 Secret key，由平台以结构化 patch 管理 `auth.userpass`，不返回完整配置或任何已有密码。
- 将 Hysteria Manager 改为无状态的平台 HTTP 客户端：发现 Hysteria2 应用、读取公开运行态、管理用户并触发发布，客户端配置仅在响应中临时生成。
- 删除 Hysteria Manager 的本地 Hysteria 二进制、YAML、进程、SQLite/状态文件和本地 profile 持久化依赖。

## Capabilities

### Added Capabilities

- `delegated-application-management`: 平台可向外部管理台授予短期、范围受限且可审计的应用管理权限。
- `controlled-structured-documents`: 应用可显式授权单个 Secret/ConfigMap key 的特定 YAML/JSON 路径由受权管理台以受控 patch 维护。
- `application-restart-release`: 平台可基于当前有效的应用声明重新发布工作负载，而不暴露或复制敏感配置。

### Modified Capabilities

- `application-capability-discovery`: capability 发现和运行态摘要可由受限委托身份调用，并保持原有浏览器 JWT 调用兼容。
- `application-release`: 应用发布支持由受控配置变更驱动的重新发布，并记录关联操作与原始用户。

## Non-goals

- 不提供项目级 Secret 列表、Secret 明文读取或通用 Kubernetes API 代理。
- 不让 Hysteria Manager 直接访问 Kubernetes、SQLite 或 Cylism Manager 的内部数据库。
- 不实现任意 YAML 文本覆盖、任意 JSON Pointer 或任意模板编辑。
- 不在本期支持多集群、跨项目委托、长期机器令牌或自动账号同步。
- 不恢复 Hysteria Manager 作为 Hysteria 进程宿主的模式。

## Impact

- Cylism Manager 增加委托令牌、受控文档元数据、受限 Integration API 和重启发布编排。
- 应用模板须显式声明可管理的文件资源，平台才允许外部管理台修改对应 Secret key。
- Hysteria Manager 的后端 API 与前端从本地文件/进程模型迁移为平台 API 模型；部署只需 Manager URL 和签名/委托配置，不需 PVC。
- 旧 Hysteria Manager 本地数据与 profile 将不再被读取；迁移前需由用户确认已把运行配置迁入应用 Secret。
