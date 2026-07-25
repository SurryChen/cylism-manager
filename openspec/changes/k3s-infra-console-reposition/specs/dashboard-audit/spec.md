## ADDED Requirements

### Requirement: 概览页引导与集群总览
系统 SHALL 在概览页展示 tailnet 连接状态、control-plane 状态、集群资源总览、待激活服务器数和最近操作日志，并在关键前置条件缺失时展示引导卡片。

#### Scenario: 缺少 tailnet 或 join token 引导
- **WHEN** 系统检测到无法读取本机 tailnet 状态，或缺少 `k3s_join_token`
- **THEN** 概览页显示引导卡片，提示用户前往“服务器”或“系统设置”完成配置

#### Scenario: 展示集群摘要
- **WHEN** 用户打开概览页
- **THEN** 页面展示节点数、命名空间数、工作负载数、服务数和异常节点数

### Requirement: 高风险操作审计覆盖
系统 SHALL 将导入服务器、更新 SSH 凭据、激活服务器、加入 worker、移除节点、查看 Secret 明文和启用扩展纳入审计日志。

#### Scenario: 激活服务器写审计
- **WHEN** 已认证用户触发服务器激活检测
- **THEN** 系统写入审计日志，包含 action、resource_type、resource_id、result 和 user_id

#### Scenario: 查看 Secret 明文写审计
- **WHEN** 已认证用户在配置页面确认查看某 Secret 的明文值
- **THEN** 系统写入一条查看敏感数据的审计日志
