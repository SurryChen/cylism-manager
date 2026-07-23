# deploy-probe Specification

## Purpose
TBD - created by archiving change deploy-probe. Update Purpose after archive.
## Requirements
### Requirement: Agent 状态探测
系统 SHALL 在部署前通过 SSH 探测远端服务器的 Agent 状态，包括进程、systemd service、二进制文件和版本号。

#### Scenario: 探测到运行中的 Agent
- **WHEN** 用户调用 POST /api/servers/:id/deploy/probe
- **THEN** 系统通过 SSH 执行探测命令，返回 installed=true, process_running=true, binary_exists=true, agent_version="1.0.0", listening_port=9527

#### Scenario: 未探测到 Agent
- **WHEN** 远端无 Agent 二进制、进程和 service
- **THEN** 系统返回 installed=false, process_running=false, binary_exists=false

#### Scenario: SSH 连接失败时阻断
- **WHEN** SSH 凭据无效或网络不通
- **THEN** 系统返回 HTTP 502 和错误信息，不继续推进部署流程

#### Scenario: 部分探测命令失败
- **WHEN** 远端 systemctl 不可用但其他探测正常
- **THEN** 系统标记 systemd_exists=false, systemd_active=false，返回其他探测项的正确结果

