## ADDED Requirements

### Requirement: 证书页面受扩展状态控制
系统 SHALL 将证书能力建模为扩展感知页面；当 cert-manager 未安装时，页面展示未启用状态与引导，而不是直接暴露宿主机 acme.sh 操作。

#### Scenario: 未安装 cert-manager
- **WHEN** 用户打开 `/certs`
- **THEN** 系统检测不到 cert-manager CRD 或核心控制器
- **AND** 页面展示“未启用证书扩展”状态和安装说明

#### Scenario: 已安装 cert-manager
- **WHEN** 用户打开 `/certs`
- **THEN** 系统检测到 cert-manager CRD 和控制器运行正常
- **AND** 页面展示 `Certificate` 资源列表与详情入口

### Requirement: 证书资源查看
系统 SHALL 在启用 cert-manager 的前提下展示 Kubernetes `Certificate` 资源的列表和详情。

#### Scenario: 查看 Certificate 列表
- **WHEN** cert-manager 已启用且用户请求证书列表
- **THEN** 系统返回每个 `Certificate` 的 name、namespace、secret_name、ready 状态、renewal_time 或到期时间

#### Scenario: 查看 Certificate 详情
- **WHEN** 用户打开某个 `Certificate` 详情
- **THEN** 系统展示 spec、status、条件和关联 Secret 名称

## REMOVED Requirements

### Requirement: 证书签发
**Reason**: V1 不再以宿主机 acme.sh 为证书主链路，证书能力迁移为集群扩展感知模式。

### Requirement: 证书续期
**Reason**: 续期责任由 cert-manager 等扩展承担，平台在 V1 主要提供资源可视化和入口能力。

### Requirement: 证书吊销
**Reason**: 宿主机 acme.sh 吊销流程不再是当前产品主路径。

### Requirement: 证书到期仪表盘
**Reason**: 该能力后续可以基于 `Certificate` 资源恢复，但不再绑定宿主机证书记录模型。
