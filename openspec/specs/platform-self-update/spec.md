# platform-self-update Specification

## Purpose
TBD - created by archiving change platform-self-update. Update Purpose after archive.
## Requirements
### Requirement: 接收签名的平台部署请求
系统 SHALL 允许 GitHub Actions 通过公开 HTTPS Webhook 提交平台镜像升级请求，而不要求平台访问 GitHub。

#### Scenario: 接收有效 GitHub Action 请求
- **WHEN** 请求包含有效时间戳、未使用 nonce、正确 HMAC 签名和允许仓库内的 image digest
- **THEN** 系统持久化 `accepted` PlatformRelease 并返回 `202 Accepted`
- **AND** 系统开始更新固定的 `default/cylism-manager` Deployment

#### Scenario: 拒绝无效或重放请求
- **WHEN** 请求签名无效、时间戳超过五分钟、nonce 已使用，或 image 不含 digest
- **THEN** 系统拒绝请求且不创建 PlatformRelease
- **AND** 系统不更新 Kubernetes Deployment

#### Scenario: 拒绝非平台镜像
- **WHEN** 请求 image 不匹配管理员配置的平台镜像仓库前缀
- **THEN** 系统拒绝请求并记录安全审计信息
- **AND** 系统不得修改任何 Kubernetes 工作负载

### Requirement: 受限的自更新与状态恢复
系统 SHALL 仅更新固定平台 Deployment 的 `platform` 容器，并在自身重启后恢复未完成发布状态。

#### Scenario: 使用不可变 digest 更新平台
- **WHEN** PlatformRelease 被接受
- **THEN** 系统将 `default/cylism-manager` 内 `platform` 容器 image 更新为请求的 `image@sha256:...`
- **AND** 系统记录更新前 image 和发布 ID annotation

#### Scenario: 自重启后的发布状态收敛
- **WHEN** PlatformRelease 已提交 Deployment 更新且旧平台 Pod 被替换
- **THEN** 新启动的平台实例从 SQLite 读取未完成记录
- **AND** 根据 Deployment 目标 image 与就绪副本更新为 `succeeded` 或 `failed`

### Requirement: 平台发布运维界面
系统 SHALL 允许管理员查看平台当前版本、发布历史、状态详情和受确认的回滚入口。

#### Scenario: 回滚到上一版本
- **WHEN** 管理员确认回滚某次失败或已完成的 PlatformRelease
- **THEN** 系统使用记录的 `previous_image` 创建新的 PlatformRelease
- **AND** 系统通过与正常自更新相同的状态收敛逻辑展示结果

