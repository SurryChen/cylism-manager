# managed-domain-lifecycle Specification

## Purpose
TBD - created by archiving change managed-domain-lifecycle. Update Purpose after archive.
## Requirements
### Requirement: Managed domain provisions a namespace-scoped certificate

系统 SHALL 将受管域名绑定到一个命名空间和已就绪的 ClusterIssuer，并为其自动维护 cert-manager Certificate 与 TLS Secret。

#### Scenario: 创建受管 HTTPS 域名
- **WHEN** 用户提交合法 Host、存在的命名空间和已就绪的 ClusterIssuer
- **THEN** 系统创建受管域名记录，并在该命名空间创建由平台命名的 Certificate 与 TLS Secret

#### Scenario: Issuer 尚未就绪
- **WHEN** 用户选择不存在或未 Ready 的 ClusterIssuer 创建受管域名
- **THEN** 系统拒绝请求，不创建域名记录或 Certificate

#### Scenario: 旧域名尚未绑定命名空间
- **WHEN** 迁移前创建的域名缺少命名空间绑定
- **THEN** 系统将其展示为未绑定，且不允许作为 HTTPS 发布入口使用

### Requirement: Domain status follows Certificate state

系统 SHALL 从对应 Certificate 读取签发状态、TLS Secret、续期时间和失败原因，而不在数据库缓存证书状态。

#### Scenario: Certificate 签发失败
- **WHEN** 受管域名对应 Certificate 的 Ready 条件为 False
- **THEN** 域名列表展示失败原因，并允许用户发起幂等重试与查看签发过程

#### Scenario: Certificate 已就绪
- **WHEN** Certificate 的 Ready 条件为 True
- **THEN** 域名列表展示 TLS Secret 和续期时间，并允许同命名空间应用使用该域名

### Requirement: HTTPS application releases reuse managed domain certificate

系统 SHALL 让 HTTPS 应用发布引用受管域名已有的 TLS Secret，而不为同一域名额外创建 Certificate。

#### Scenario: 发布到匹配命名空间
- **WHEN** 应用环境命名空间与选中受管域名命名空间一致，且其 Certificate 已就绪
- **THEN** 系统创建 Ingress 并引用受管域名 TLS Secret，不创建新的 Certificate

#### Scenario: 命名空间不匹配或证书未就绪
- **WHEN** 应用选择不同命名空间的域名，或 TLS Certificate 尚未 Ready
- **THEN** 系统拒绝 HTTPS 发布并返回明确原因

### Requirement: Managed domain deletion is reference-safe

系统 SHALL 在删除受管域名前检查应用入口引用关系。

#### Scenario: 删除被引用域名
- **WHEN** 域名仍被任一应用入口引用
- **THEN** 系统返回冲突并拒绝删除

#### Scenario: 删除未引用域名
- **WHEN** 域名没有应用入口引用
- **THEN** 系统删除受管域名对应 Certificate 和数据库记录

### Requirement: Business DNS is distinct from ACME DNS-01

系统 SHALL 明确区分 ACME DNS-01 TXT 验证与业务流量 DNS 解析。

#### Scenario: 查看已申请的域名
- **WHEN** 用户查看受管域名详情
- **THEN** 系统展示业务 DNS 解析仍需指向集群入口的提示，且不将临时 ACME TXT 记录误表示为业务解析

