# Design: Application Capability Discovery

## Data Model

`Application` 新增 `Capabilities string` 持久化字段，使用 JSON 字符串数组存储。API 仅暴露 `[]string`。保存时会：去掉首尾空白、去重、按字典序排序，并限制每个标识为小写 ASCII、数字和连字符，最大 64 个字符；单个应用最多 16 项。

能力归属于 Application 而非 Deployment Template：同一应用的多个模板可能改变镜像、端口或资源，但不应让管理权限和发现结果随着模板切换而改变。平台不保留 capability 注册表，也不对 `hysteria2` 等具体值做分支判断。

对历史 Application，空字段映射为空数组，保持现有发布与读取行为。

## APIs

在既有 JWT 保护的 `/api/applications` 下增加：

- `PUT /:id/capabilities`：替换指定应用的 capability 列表。它只校验 Application 存在与输入合法性；现有平台目前没有项目级用户角色，因此与其它 Application 编辑端点保持相同的认证边界。
- `GET /discovery?project_id=&environment_id=&capability=`：要求有效项目 ID；环境 ID 可选但必须属于项目；capability 可选。返回应用基础信息、capabilities、环境、端点和运行态摘要，不返回模板 Spec 或 Release `DesiredSpec`。
- `GET /:id/runtime`：返回一个应用的同等运行态摘要。该应用必须存在；查询通过其 Environment 推导 Namespace。

发现响应只包含 UI/API 生成客户端配置所需的公开元数据：Service 名称、类型、端口、协议、NodePort、LoadBalancer 地址、公开 Endpoint 域名与路径、latest release 的 ID/sequence/status/version、期望/就绪副本，以及 ready Pod 的名称和 nodeName。Secret 名称、TLS Secret、环境变量、文件挂载、加密字段和 Secret 内容都不出现在响应中。

## Kubernetes Runtime Collection

运行态查询使用 Application 的 Namespace 和名称：

1. 获取同名 Service，映射所有 Service ports 以及 `.status.loadBalancer.ingress` 的 IP/hostname。
2. 获取同名 Deployment 或 StatefulSet，优先匹配 Application 的 `workload_kind`，读取 spec replicas、status readyReplicas 与 availableReplicas。
3. 使用平台稳定的 `app.kubernetes.io/name` 标签列出 Pod，仅返回 Ready 的 Pod 名称与 `spec.nodeName`。结果按名称排序并去重节点。
4. Kubernetes 不可用或单项资源不存在时，返回基础数据库信息及对应空摘要/`unavailable` 状态，避免整个发现列表失败。

为避免工作台列表触发 N+1 Kubernetes 请求，`GET /discovery` 在一个 Namespace 内批量 List Service、Deployment、StatefulSet 与 Pod，然后按应用名汇总。单应用 API 可以复用相同汇总函数。

## UI Scope

本阶段在既有 Application 详情页增加紧凑的“能力标签”编辑区域，使用可增删的行输入，保存到 `PUT /:id/capabilities`。应用列表和工作台只显示 capability 标签，不以应用类型创建新的页面或专用逻辑。

## Security Boundary

本阶段不会开放跨服务机器身份认证。`cylism-hysteria-manager` 后续必须使用短期、项目范围、动作范围受限的委托凭据；该凭据的签发、轮换和审计与写操作一起在下一阶段引入。

## Risks And Mitigations

- 运行态数据很慢：发现 API 每个 Namespace 批量查询 Kubernetes，并将资源缺失降级为局部状态。
- capability 被误用作授权：它只用于发现筛选；后续仍以委托凭据的项目与应用范围授权。
- Service 或 Pod 元数据泄漏：响应限定在应用自有 Namespace，且 DTO 明确排除 Secret/Spec 内容。
- 历史空数据影响发布：空数组兼容，能力标签不参与渲染或发布。

## Alternatives Considered

- 在 Deployment Template 保存 capability：模板切换会改变外部管理面的发现和权限边界，拒绝采用。
- 由镜像名称自动识别 Hysteria2：镜像可重命名、标签可复用，也无法覆盖其他协议应用，拒绝采用。
- 让管理器直连 Kubernetes API：需要重复 RBAC、审计和资源归属逻辑，破坏平台作为唯一控制面，拒绝采用。
- 返回 TLS Secret 或普通 Secret 值给管理器：浏览器与无状态服务不应持有项目全部秘密，拒绝采用。
