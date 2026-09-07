## ADDED Requirements

### Requirement: 本机 tailnet 状态读取
系统 SHALL 通过 control-plane 宿主机暴露的 `tailscaled` socket 读取本机 tailnet 状态。

#### Scenario: 读取本机状态
- **WHEN** 用户打开服务器页中的导入入口或系统设置中的组网配置区
- **THEN** 系统返回本机是否在线、control-plane 的 Tailscale IPv4、peer 数量和 socket 可用性

#### Scenario: 无法访问 socket
- **WHEN** 平台无法访问宿主机 `tailscaled` socket
- **THEN** 系统返回明确错误，并提示检查部署挂载前提

### Requirement: 保存可复用的 Tailscale Auth Key
系统 SHALL 允许用户保存可复用的 Tailscale auth key，供远程服务器在加入集群前补装或登录 tailnet。

#### Scenario: 保存 Auth Key
- **WHEN** 用户在系统设置中提交新的 Tailscale auth key
- **THEN** 系统加密存储该 key，并仅返回脱敏摘要

#### Scenario: 读取 Auth Key 摘要
- **WHEN** 用户查询当前 Tailscale auth key 状态
- **THEN** 系统返回脱敏摘要，而不是明文

### Requirement: tailnet peer 导入预览
系统 SHALL 在正式导入前生成 peer 导入预览，区分可导入、已存在和冲突项。

#### Scenario: 预览包含冲突项
- **WHEN** 某个 peer 的 device_id 或候选管理地址已存在于服务器台账
- **THEN** 预览结果中将该 peer 标记为 `conflict`

#### Scenario: 只导入无冲突项
- **WHEN** 用户选择“仅导入无冲突项”并确认
- **THEN** 系统仅创建 `importable` 分类下的服务器记录
