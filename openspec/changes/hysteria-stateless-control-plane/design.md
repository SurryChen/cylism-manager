## Context

Cylism Manager 是应用、模板、Kubernetes 资源、Secret、Release 与审计的唯一控制面。Hysteria Manager 应当提供该领域的管理体验，但不应重复持有基础设施权限或敏感配置。

一期的 `application-capability-discovery` 已让外部管理台以 `hysteria2` capability 发现项目内应用并读取脱敏运行态。本变更补齐写路径：授权、资源范围、并发控制、重新发布和无状态 Hysteria Manager 迁移。

## Goals / Non-goals

**Goals:**

- 外部管理台只能代表某一已认证用户，在短时间和受限项目范围内调用平台。
- 管理台只能修改一个应用预先声明的 Secret/ConfigMap key 的允许路径，永不读取完整文档或既有敏感值。
- 修改成功后可安全触发该应用基于当前期望声明的重启发布，并查询状态。
- Hysteria2 用户创建/重置时仅一次性返回生成密码和客户端配置，不持久化 profile。
- Hysteria Manager 不需要本地磁盘、SQLite、Hysteria 二进制或 Kubernetes 凭据。

**Non-goals:**

- 不以 capability 作为授权本身；它只作为令牌范围和发现过滤条件。
- 不允许外部管理台为未标记的应用创建受控资源或修改模板。
- 不返回 Secret key 的原文、其他用户账号、TLS 私钥或 `trafficStats.secret`。
- 不以 Release 快照中的脱敏数据重建 Secret；所有发布均复用当前受平台管理的应用资源引用。

## Decisions

### 1. 独立的委托令牌而非复用浏览器 JWT

平台新建 `DelegationClaims`，包含 `typ=delegation`、`aud=cylism-integration`、原始 `user_id`、允许的 `project_id`、`environment_ids`、`capability`、`actions`、`iat`、`exp`、唯一 `jti`。采用现有 HMAC 签名设施但使用专用 token type、audience 和不超过 10 分钟的有效期；解析时必须验证全部声明。

浏览器用户先在平台中选择项目和目标管理台，平台基于其现有 JWT 与当前权限签发 delegation。Hysteria Manager 将该 token 放入后续平台请求的 `Authorization: Bearer`。平台根据 claims 检查每个资源的项目、环境和 application capability，并以 claim 的原始 user ID 写审计记录。令牌不含 Secret、模板或 Kubernetes 凭据。

Hysteria Manager 可将 delegation 保存为加密签名的短期 HttpOnly 会话 cookie，或由浏览器在每次操作时传入；它不得把 token 写入数据库或文件。第一版采用无服务器会话状态的签名 cookie，过期时间不得晚于 delegation。

### 2. 受控结构化文档是显式应用绑定

新增持久化 `ManagedDocument` 元数据，字段至少包括：ID、ApplicationID、ResourceKind (`secret`/`configmap`)、ResourceName、Key、Format (`yaml`/`json`)、AllowedPaths JSON、Version、Enabled、创建/更新时间。只有应用所在 namespace、且应用模板的一个只读文件挂载精确引用该 source/name/key 时，才能创建或启用该记录。一个 application/resource/key 至多绑定一个有效 managed document。

`AllowedPaths` 是规范化 JSON Pointer 前缀列表；它不允许空根路径、通配符、`..` 和重复项。平台仅提供返回结构/元数据的摘要，例如可管理 username 列表或指定非敏感字段；不会把受控文档全文返回给委托调用者。

外部调用提交 `operations`（`add`、`replace`、`remove`）与 `expected_version`。服务端从 Kubernetes 读取该指定 key，使用 YAML/JSON parser 解析，在每个目标路径均落在 `AllowedPaths` 内时才变更，重新序列化并更新同一 Secret/ConfigMap key；冲突返回当前版本号但不返回内容。Secret 内容不得写入 Release、OperationLog、错误文本或 HTTP 响应。

Hysteria2 只申请一个 Secret 文档 `config.yaml`，允许路径限定为 `/auth/userpass`。用户列表摘要仅返回 username；新增或重置密码由 Hysteria Manager 生成，平台不回显已保存密码。

### 3. 配置变更与发布解耦，但由同一操作编排

受控 patch 成功后，调用方可创建 `restart` 任务。平台校验目标应用与委托范围后，基于当前应用模板及受管理资源引用创建新的 Release，增加重启/配置版本 annotation 或相等的 Pod-template 变更，使 Kubernetes 重建工作负载。不得从脱敏 `DesiredSpec` 恢复 Secret 值，也不得重新创建受控 Secret。

为避免“配置已改但未重启”不可见，外部管理台的用户写操作提供原子工作流：`PATCH` 成功后立即创建 restart Release，响应返回 document version 和 release ID。若 restart 创建失败，配置保留并返回明确的 `restart_pending` 状态，管理台可在权限范围内重试重启。发布状态沿用已有 Release 查询/轮询接口。

### 4. Integration API 与现有浏览器 API 分层

新增 `/api/integrations/applications` 路由组，仅接受 delegation。路由包括：

- `GET /discovery?project_id=&environment_id=&capability=`：复用脱敏发现 DTO。
- `GET /:id/runtime`：复用脱敏运行态 DTO。
- `GET /:id/managed-documents`：仅返回已授权文档的 ID、格式、版本、允许路径和领域摘要。
- `PATCH /:id/managed-documents/:documentID`：受限结构化 patch，并可选 `restart=true`。
- `POST /:id/restarts`：创建不改变镜像/模板的重启 Release。
- `GET /:id/releases/:releaseID`：查询该应用 Release 的脱敏状态。

普通 `/api/applications` JWT 路由保持兼容，可由平台 UI 管理 ManagedDocument；Integration 路由不能访问通用 Secret CRUD、模板读取或全量 Release DesiredSpec。

### 5. Hysteria Manager 的无状态领域映射

Hysteria Manager 后端替换本地 `Manager`：它只持有 HTTP client、平台 URL、会话签名配置和纯函数式 profile renderer。应用概览从 discovery/runtime 获取；用户列表读 ManagedDocument 的摘要；添加/删除/重置用户提交受控 patch 和 restart 并轮询 Release。

客户端 profile 由公开运行态的 UDP Service 地址/端口、域名/SNI 与本次创建/重置返回的密码组合生成，直接作为下载响应，不存入文件。无法确定公开地址时，界面显示可编辑/复制的服务端地址字段，绝不推测私有 ClusterIP。

删除本地 `config.go`、进程生命周期、`state.json`、profile 文件写入、流量 API 和运行时二进制路径；Health API 只反映自身及必要的平台连通性。

## Data and Migration

1. 升级 Cylism Manager，自动创建 `managed_documents` 表，但不创建任何可写绑定。
2. 管理员或应用拥有者在应用配置中为已存在的 Secret 文件挂载显式创建 ManagedDocument，设置 `/auth/userpass` 等允许路径。
3. 升级 Hysteria Manager。它首次使用时只发现已有 `hysteria2` 应用；无 binding 的应用显示“尚未授权管理配置”，不会尝试读写本地文件。
4. 完成迁移确认后删除旧 Hysteria Manager 运行目录、二进制和环境变量文档；不自动删除用户数据或 Kubernetes Secret。

## Risks and Mitigations

- [委托 token 被重放] -> 短 TTL、audience/type/action/scope 验证、HttpOnly Secure cookie、审计 jti；后续可加入 jti denylist。
- [路径绕过修改其他 Secret 字段] -> 服务端结构化解析、规范 JSON Pointer、严格前缀校验、固定 resource/key binding 和乐观并发版本。
- [Secret 进入日志或 Release] -> 响应 DTO、错误处理与审计 payload 使用 redaction；测试断言不包含 secret value。
- [配置修改后 Pod 不刷新] -> patch workflow 默认请求 restart；响应明确返回 Release ID 和 pending 状态。
- [错误的服务地址生成配置] -> 只使用 Runtime DTO 的 Service 外部地址/端口或用户确认的域名，不使用 ClusterIP。

## Alternatives Considered

- 让 Hysteria Manager 挂载项目级 Secret：会泄露无关配置，且无法进行应用级审计，拒绝。
- 给 Hysteria Manager Kubernetes ServiceAccount：重复平台 RBAC 和发布逻辑，扩大权限，拒绝。
- 通过 ConfigMap 存 Hysteria `config.yaml`：账号和 API secret 属于敏感数据，默认使用 Secret；抽象仍支持 ConfigMap 以服务其他非敏感场景。
- 直接 PUT 整份 YAML：易覆盖 TLS、带宽等字段且无法限制范围，拒绝。
- 使用本地 SQLite 保存用户与 profile：会与 Kubernetes 实际配置分叉，且不满足无状态部署，拒绝。
