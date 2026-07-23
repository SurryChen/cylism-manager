## 新增需求

### Requirement: 服务器注册
系统应当允许用户注册远程服务器，提供名称、主机地址、gRPC 端口、SSH 主机、SSH 端口、SSH 用户名以及 SSH 认证凭据（密码或私钥）。

#### Scenario: 使用密码认证注册服务器
- **WHEN** 用户提交服务器注册，名称为 "web-01"，主机为 "10.0.0.1"，SSH 用户为 "root"，SSH 密码为指定值
- **THEN** 系统存储服务器记录，并在持久化到 SQLite 前使用 AES-256 加密密码

#### Scenario: 使用密钥认证注册服务器
- **WHEN** 用户提交服务器注册，附带 SSH 私钥内容
- **THEN** 系统存储服务器记录，并在持久化前使用 AES-256 加密私钥

#### Scenario: 拒绝重复主机
- **WHEN** 用户尝试注册一个已存在的主机地址
- **THEN** 系统返回错误，提示该主机已注册

### Requirement: Agent 部署
系统应当通过 SSH 将 Agent 二进制部署到已注册的服务器，探测远程操作系统和架构，上传匹配的二进制文件，并启动 Agent 进程。

#### Scenario: Agent 部署成功
- **WHEN** 用户对一台具有有效 SSH 凭据的服务器触发部署
- **THEN** 系统通过 SSH 连接，检测 OS/Arch，上传 Agent 二进制，通过 systemd 或 nohup 启动 Agent，建立 gRPC 连接，并将服务器状态设为 "online"

#### Scenario: SSH 认证失败
- **WHEN** 用户触发部署但 SSH 凭据无效
- **THEN** 系统返回认证失败的错误，并将服务器状态设为 "offline"

#### Scenario: 远程未安装 acme.sh
- **WHEN** Agent 在未安装 acme.sh 的服务器上启动
- **THEN** 系统将服务器标记为 online，但在服务器详情中将 acme.sh 标记为不可用

### Requirement: 服务器状态监控
系统应当与每个 Agent 维持 gRPC 心跳，并据此更新服务器状态。

#### Scenario: Agent 心跳正常
- **WHEN** Agent 在 30 秒超时内响应 Ping RPC
- **THEN** 服务器状态保持 "online"，并更新 last_seen 时间戳

#### Scenario: Agent 心跳丢失
- **WHEN** Agent 连续 3 次未能响应 Ping RPC
- **THEN** 系统将服务器状态设为 "offline"

### Requirement: 服务器列表与详情
系统应当提供 API 端点以列出所有已注册服务器，以及查看单个服务器详情（含状态和最后在线时间）。

#### Scenario: 列出所有服务器
- **WHEN** 用户请求服务器列表
- **THEN** 系统返回所有已注册服务器及其状态、主机和最后在线时间

#### Scenario: 查看服务器详情
- **WHEN** 用户请求特定服务器的详情
- **THEN** 系统返回完整服务器信息，包含 SSH 配置（凭据已脱敏）、Agent 状态和关联站点数量
