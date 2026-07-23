## Purpose
服务器注册、Agent 部署和状态监控管理。
## Requirements
### Requirement: 服务器注册
系统 SHALL允许用户注册远程服务器，提供名称、主机地址、gRPC 端口、SSH 主机、SSH 端口、SSH 用户名以及 SSH 认证凭据（密码或私钥）。

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
系统 SHALL通过 SSH 将 Agent 二进制部署到已注册的服务器，探测远程操作系统和架构，上传匹配的二进制文件，并启动 Agent 进程。

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
系统 SHALL与每个 Agent 维持 gRPC 心跳，并据此更新服务器状态。

#### Scenario: Agent 心跳正常
- **WHEN** Agent 在 30 秒超时内响应 Ping RPC
- **THEN** 服务器状态保持 "online"，并更新 last_seen 时间戳

#### Scenario: Agent 心跳丢失
- **WHEN** Agent 连续 3 次未能响应 Ping RPC
- **THEN** 系统将服务器状态设为 "offline"

### Requirement: 服务器列表与详情
系统 SHALL提供 API 端点以列出所有已注册服务器，以及查看单个服务器详情（含状态和最后在线时间）。

#### Scenario: 列出所有服务器
- **WHEN** 用户请求服务器列表
- **THEN** 系统返回所有已注册服务器及其状态、主机和最后在线时间

#### Scenario: 查看服务器详情
- **WHEN** 用户请求特定服务器的详情
- **THEN** 系统返回完整服务器信息，包含 SSH 配置（凭据已脱敏）、Agent 状态和关联站点数量

### Requirement: 操作日志展示
系统 SHALL在服务器详情面板中展示操作日志，支持实时轮询更新。

#### Scenario: 服务器详情面板展示操作日志
- **WHEN** 用户打开服务器详情面板
- **THEN** 系统加载并展示该服务器的操作日志列表，按时间倒序排列，每条日志显示步骤名称、状态图标（加载中/成功/失败）、详情和时间

#### Scenario: 实时轮询操作日志
- **WHEN** 服务器详情面板中有 running 状态的操作日志
- **THEN** 系统每 2 秒轮询 GET /api/operations 接口，刷新日志列表

#### Scenario: 停止轮询
- **WHEN** 所有操作日志状态均为 success 或 failed，或用户关闭详情面板
- **THEN** 系统停止轮询

### Requirement: 部署前探测确认
系统 SHALL 在用户触发部署时先展示探测结果，用户确认后才执行部署。

#### Scenario: 无 Agent 时确认部署
- **WHEN** 探测结果显示远端未安装 Agent，且用户点击"确认部署"
- **THEN** 系统执行全新部署流程

#### Scenario: 有 Agent 时覆盖确认
- **WHEN** 探测结果显示远端已有运行中的 Agent，且用户点击"覆盖部署"
- **THEN** 系统以 force=true 模式执行部署，先停止旧 Agent，再覆盖二进制并重启

#### Scenario: 用户取消部署
- **WHEN** 探测结果弹窗中用户点击"取消"
- **THEN** 系统不执行任何部署操作

### Requirement: Force 覆盖部署
系统 SHALL 支持 force 参数，在覆盖模式下先停止旧 Agent 再部署新版本。

#### Scenario: Force 覆盖已运行的 Agent
- **WHEN** 部署请求携带 force=true，且远端有运行中的 Agent
- **THEN** 系统先执行 systemctl stop（或 pkill），再覆盖二进制文件，最后重启 Agent 并建立 gRPC 连接

### Requirement: Agent 信息卡片展示
系统 SHALL 在服务器详情面板中展示 Agent 信息卡片，包含状态、版本、部署路径、最后部署时间和最后在线时间。

#### Scenario: 已部署 Agent 的服务器详情
- **WHEN** 用户点击已部署 Agent 的服务器行
- **THEN** 详情面板展示 Agent 信息卡片：在线/离线状态、Agent 版本、部署路径、最后部署时间和最后在线时间

#### Scenario: 未部署 Agent 的服务器详情
- **WHEN** 用户点击未部署 Agent 的服务器行
- **THEN** 详情面板展示 Agent 信息卡片，版本和部署路径显示为"未部署"

### Requirement: 状态同步操作入口
系统 SHALL 在服务器列表每行提供"状态同步"按钮。

#### Scenario: 点击状态同步
- **WHEN** 用户点击某服务器的"状态同步"按钮
- **THEN** 系统调用 ProbeAgent 探测远端 Agent，若运行中则更新状态为 online 并刷新列表

