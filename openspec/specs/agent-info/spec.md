# agent-info Specification

## Purpose
TBD - created by archiving change agent-info. Update Purpose after archive.
## Requirements
### Requirement: Agent 元信息记录
系统 SHALL 在 Server 表中持久化 Agent 的版本、部署路径和最后部署时间。

#### Scenario: 部署成功后写入 Agent 信息
- **WHEN** 用户成功部署 Agent
- **THEN** 系统更新 Server 记录的 agent_version、agent_deploy_path 为对应值，agent_deployed_at 为当前时间

#### Scenario: 新注册服务器无 Agent 信息
- **WHEN** 用户新注册一台服务器但未部署 Agent
- **THEN** 该服务器的 agent_version、agent_deploy_path、agent_deployed_at 均为空

### Requirement: Agent 状态同步
系统 SHALL 支持手动触发状态同步，当探测到 Agent 运行时自动将服务器状态更新为 online 并同步版本号。

#### Scenario: 状态同步将 offline 恢复为 online
- **WHEN** 服务器状态为 offline，但 agent 进程实际在运行，用户触发状态同步
- **THEN** 系统将服务器状态更新为 online，更新 agent_version 和 last_seen

#### Scenario: 状态同步确认 online
- **WHEN** 服务器状态为 online，agent 正常运行，用户触发状态同步
- **THEN** 系统更新 agent_version 和 last_seen，状态保持 online

