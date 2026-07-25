## ADDED Requirements

### Requirement: 加入集群前置检测
系统 SHALL 在将服务器加入 K3s 集群前执行前置条件检测，所有检测通过后方可继续。

#### Scenario: 所有检测通过
- **WHEN** SSH 可达、用户为 root、swap 已关闭、OS 兼容（x86_64/aarch64 + systemd）、磁盘 ≥ 2GB
- **THEN** 返回 `all_pass: true`，各检测项 `pass: true`

#### Scenario: swap 未关闭
- **WHEN** 远程服务器 swap 处于开启状态
- **THEN** 返回 `all_pass: false`，`swap_disabled` 检测项 `pass: false`，detail 显示 swap 占用详情

#### Scenario: 非 root 用户
- **WHEN** SSH 用户 uid 不为 0
- **THEN** 返回 `all_pass: false`，`root_privilege` 检测项 `pass: false`

#### Scenario: SSH 不可达
- **WHEN** SSH 连接失败
- **THEN** 返回 `all_pass: false`，`ssh_connect` 检测项 `pass: false`，后续检测项仍尝试执行（best-effort）
