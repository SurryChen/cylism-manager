## Purpose
服务器注册、远程凭据管理、Agent 部署和状态监控管理，为平台纳管主机提供统一台账、连接能力、运行事实和生命周期操作。
## Requirements
### Requirement: 服务器注册
系统 SHALL 允许用户注册远程 Linux 服务器，提供名称、管理地址、SSH 端口、SSH 用户、权限模式以及 SSH 认证凭据（密码或私钥）；系统 SHALL 不再要求 gRPC 端口或 Agent 部署信息。

#### Scenario: 使用密码认证注册服务器
- **WHEN** 用户提交服务器注册，名称为 "worker-01"，管理地址为 "100.101.1.10"，SSH 用户为 "ops"，权限模式为 "sudo"，SSH 密码为指定值
- **THEN** 系统存储服务器记录，并在持久化前使用 AES-256 加密密码
- **AND** 新记录状态为 `credential_pending` 或 `ready` 之前的纳管状态，而不是 Agent online/offline 状态

#### Scenario: 使用密钥认证注册服务器
- **WHEN** 用户提交服务器注册，附带 SSH 私钥内容与可选口令
- **THEN** 系统存储服务器记录，并在持久化前加密私钥和口令

#### Scenario: 拒绝重复管理地址
- **WHEN** 用户尝试注册一个已存在的 `management_address`
- **THEN** 系统返回错误，提示该服务器已注册

### Requirement: 服务器列表与详情
系统 SHALL 提供服务器列表与详情能力，返回服务器台账、Tailscale 纳管信息、激活状态和主机事实缓存摘要；系统 SHALL 不再把 Agent 心跳状态作为列表主状态。

#### Scenario: 列出所有服务器
- **WHEN** 用户请求服务器列表
- **THEN** 系统返回名称、管理地址、`tailscale_ipv4`、`tailscale_online`、`ssh_user`、`privilege_mode`、`activation_state`、`last_collected_at`
- **AND** 响应中不包含明文凭据

#### Scenario: 查看服务器详情
- **WHEN** 用户请求特定服务器详情
- **THEN** 系统返回完整台账信息、脱敏后的 SSH 配置、最近一次激活结果和主机事实缓存

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

### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: Tailnet 服务器导入
系统 SHALL 支持从当前 control-plane 宿主机可见的 tailnet peer 列表中预览并导入服务器记录。

#### Scenario: 预览可导入服务器
- **WHEN** 用户在服务器页的导入入口触发“扫描 tailnet 设备”
- **THEN** 系统返回 peer 列表，并将结果区分为 `importable`、`existing`、`conflict`

#### Scenario: 严格模式导入失败
- **WHEN** 用户以严格模式提交导入，且预览结果中存在冲突项
- **THEN** 系统拒绝本次导入，并返回冲突列表

#### Scenario: 导入无冲突服务器
- **WHEN** 用户确认导入所有无冲突 peer
- **THEN** 系统批量创建服务器记录
- **AND** 新记录至少包含 `tailscale_device_id`、`tailscale_hostname`、`tailscale_ipv4`

### Requirement: 服务器激活检测
系统 SHALL 通过 SSH 对服务器执行激活检测，并将结果映射到纳管状态。

#### Scenario: 激活检测通过
- **WHEN** 服务器 SSH 登录成功，且 root 或 sudo、systemd、磁盘空间等前置条件均满足
- **THEN** 系统将服务器状态更新为 `ready`

#### Scenario: SSH 登录失败
- **WHEN** 服务器凭据错误或网络不可达
- **THEN** 系统将服务器状态更新为 `connectivity_failed`
- **AND** 详情中记录最近一次失败原因

### Requirement: 服务器实时资源查看
系统 SHALL 在服务器详情中支持按需查看实时资源使用情况，并采用短缓存避免重复 SSH 采集。

#### Scenario: 首次打开详情触发采集
- **WHEN** 用户打开某服务器详情并请求实时资源
- **THEN** 系统通过 SSH 采集 CPU、内存、磁盘和负载信息并返回

#### Scenario: 短时间内重复查看命中缓存
- **WHEN** 同一服务器在 15 秒内重复请求实时资源
- **THEN** 系统返回缓存结果，而不是再次建立 SSH 会话
