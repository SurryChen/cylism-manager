# stateless-application-management Specification

## Purpose
TBD - created by archiving change hysteria-stateless-control-plane. Update Purpose after archive.
## Requirements
### Requirement: Scoped delegated application access

平台 SHALL 向外部管理台签发短期、签名、受限范围的委托凭据，并仅接受该凭据访问 Integration API。

#### Scenario: 签发受限 delegation

- **WHEN** 已认证用户为指定项目、环境范围、capability 和动作签发 delegation
- **THEN** 平台 SHALL 签发带有独立 type、audience、用户、项目、环境、capability、动作、iat、exp 与 jti 的签名令牌
- **AND THEN** 令牌有效期 SHALL 不超过十分钟

#### Scenario: 拒绝错误或过期 delegation

- **WHEN** Integration API 收到签名无效、已过期、type/audience 不匹配或缺少所需 action 的令牌
- **THEN** 平台 SHALL 拒绝请求
- **AND THEN** 平台 SHALL 不执行 Kubernetes、文档或 Release 写操作

#### Scenario: 强制项目和 capability 范围

- **WHEN** delegation 调用目标应用不属于 token 的项目/环境范围，或不包含 token 指定 capability
- **THEN** 平台 SHALL 拒绝请求且不返回目标应用的信息

#### Scenario: 审计委托写操作

- **WHEN** delegation 成功修改受控文档或创建 restart Release
- **THEN** 平台 SHALL 以 token 中的原始用户身份记录审计
- **AND THEN** 审计记录 SHALL 包含 integration、application、document/release 和 delegation jti 标识
- **AND THEN** 审计记录 SHALL 不包含 Secret 内容或密码

### Requirement: Application-controlled structured documents

平台 SHALL 允许应用显式授权一个已挂载 Secret 或 ConfigMap key 的受限结构化管理，而不暴露文档全文。

#### Scenario: 创建有效的受控文档绑定

- **WHEN** 应用拥有者为应用同 namespace 中、被模板以只读文件挂载精确引用的 Secret/ConfigMap name/key 创建 YAML 或 JSON 受控文档
- **THEN** 平台 SHALL 保存 application、resource kind/name/key、format、allowed paths 和版本元数据

#### Scenario: 拒绝未挂载或越界资源

- **WHEN** 请求绑定跨 namespace 资源、未被应用模板挂载的 key、无效格式或空/非法 allowed path
- **THEN** 平台 SHALL 拒绝绑定且不创建可写入口

#### Scenario: 返回脱敏文档摘要

- **WHEN** 有效 delegation 请求目标应用的受控文档列表
- **THEN** 平台 SHALL 仅返回文档标识、格式、版本、允许路径和领域定义的非敏感摘要
- **AND THEN** 响应 SHALL 不包含 Secret/ConfigMap 全文、TLS 私钥、认证密码或未授权路径值

### Requirement: Controlled structured mutation

平台 SHALL 仅以服务端验证的结构化 patch 修改受控文档，强制路径范围、版本和资源绑定。

#### Scenario: 修改允许的 YAML 路径

- **WHEN** 有 `managed_document:write` action 的有效 delegation 提交匹配 expected version 的 add、replace 或 remove 操作，且每个 JSON Pointer 落在 document allowed paths 内
- **THEN** 平台 SHALL 从绑定的资源/key 读取文档、结构化解析、应用 patch 并仅写回该 key
- **AND THEN** 平台 SHALL 增加 document version 并返回新版本而不返回敏感内容

#### Scenario: 拒绝越权 patch

- **WHEN** patch 的路径位于 allowed paths 外、包含非法 JSON Pointer、尝试替换 document 根、操作不受支持或 delegation 缺少 write action
- **THEN** 平台 SHALL 拒绝 patch
- **AND THEN** Kubernetes 资源与 document version SHALL 保持不变

#### Scenario: 处理并发修改

- **WHEN** patch 的 expected version 与当前 document version 不一致
- **THEN** 平台 SHALL 返回冲突和当前版本号
- **AND THEN** 平台 SHALL 不写入新的文档内容

#### Scenario: Hysteria userpass 管理

- **WHEN** Hysteria2 应用将其 Secret `config.yaml` 绑定为仅允许 `/auth/userpass` 的 YAML 文档
- **THEN** 受权 Hysteria 管理台 SHALL 能添加、重置或删除该 map 下的用户名
- **AND THEN** 用户列表摘要 SHALL 只返回用户名而不返回已有密码

### Requirement: Delegated restart release

平台 SHALL 允许有明确 action 的 delegation 在不暴露配置的情况下重新发布其范围内应用。

#### Scenario: 配置修改后创建 restart Release

- **WHEN** 有效受控 patch 请求 `restart=true` 且 delegation 包含 `application:restart`
- **THEN** 平台 SHALL 创建与该应用关联的 restart Release
- **AND THEN** 响应 SHALL 返回 document version、release ID 和初始状态

#### Scenario: 从当前有效声明重新发布

- **WHEN** 平台执行 restart Release
- **THEN** 平台 SHALL 使用当前应用模板和现有受管理资源引用重新应用工作负载
- **AND THEN** 平台 SHALL 使 Pod template 发生可追踪的重启变更
- **AND THEN** 平台 SHALL 不从脱敏 Release snapshot 恢复或返回 Secret 值

#### Scenario: 查询委托创建的 Release

- **WHEN** 有效 delegation 查询其范围内应用的 restart Release
- **THEN** 平台 SHALL 返回脱敏的 Release 状态、步骤与错误摘要
- **AND THEN** 响应 SHALL 不包含 DesiredSpec 中的敏感字段或 Secret 内容

### Requirement: Stateless Hysteria Manager integration

Hysteria Manager SHALL 作为无状态外部管理台，通过 delegation 使用平台 API 管理 Hysteria2 应用。

#### Scenario: 发现 Hysteria2 应用

- **WHEN** Hysteria Manager 收到带有效 delegation 的项目概览请求
- **THEN** 它 SHALL 调用平台 discovery/runtime Integration API，并只展示 `hysteria2` capability 范围内的公开运行态
- **AND THEN** 它 SHALL 不访问 Kubernetes API、项目 Secret 或本地 Hysteria 配置

#### Scenario: 一次性生成客户端配置

- **WHEN** 用户新建或重置 Hysteria 用户成功，且平台已接受用户密码 patch
- **THEN** Hysteria Manager SHALL 仅在当前响应中生成客户端配置和密码
- **AND THEN** 它 SHALL 不将密码或 profile 持久化到本地磁盘、数据库或日志

#### Scenario: 无本地状态启动

- **WHEN** Hysteria Manager 在没有数据卷、Hysteria 二进制、配置文件或可写本地目录的容器中启动
- **THEN** 服务 SHALL 可提供健康检查和委托平台访问能力
- **AND THEN** 其运行不依赖 SQLite、state.json、用户 YAML 或本地 Hysteria 子进程

