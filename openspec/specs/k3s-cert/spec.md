# k3s-cert Specification

## Purpose
TBD - created by archiving change k3s-native-arch. Update Purpose after archive.
## Requirements
### Requirement: Certificate 列表
系统 SHALL 展示集群中 cert-manager Certificate 资源及其状态。

#### Scenario: 查看证书列表
- **WHEN** 用户访问证书管理页面
- **THEN** 系统列出所有 Certificate，包含域名、到期时间、状态（Ready/Issuing/Failed）

### Requirement: Certificate 创建
系统 SHALL 支持创建 cert-manager Certificate 资源，自动通过 Let's Encrypt 签发。

#### Scenario: 创建证书
- **WHEN** 用户提交域名和 Issuer 名称
- **THEN** 系统创建 Certificate CRD，cert-manager 自动发起 ACME 挑战并签发

### Requirement: Certificate 删除
系统 SHALL 支持删除 Certificate 及关联的 TLS Secret。

#### Scenario: 删除证书
- **WHEN** 用户确认删除证书
- **THEN** 系统删除 Certificate CRD，关联的 TLS Secret 同步清理

