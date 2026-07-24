## ADDED Requirements

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
