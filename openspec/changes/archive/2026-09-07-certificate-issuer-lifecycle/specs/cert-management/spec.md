# Certificate Management Delta

## ADDED Requirements

### Requirement: Issuer lifecycle management

系统 SHALL 管理 cert-manager Issuer 与 ClusterIssuer，并支持自签名、ACME HTTP-01 与 ACME AliDNS DNS-01 签发模式。

#### Scenario: 创建 HTTP-01 ClusterIssuer
- **WHEN** 用户提交名称、邮箱和 HTTP Ingress Class 为 `traefik` 的 ACME HTTP-01 ClusterIssuer
- **THEN** 系统创建带有 HTTP-01 solver 的 ClusterIssuer，并返回其实际 Ready 状态与失败原因

#### Scenario: 创建命名空间级 Issuer
- **WHEN** 用户选择 Issuer 类型并提交目标命名空间
- **THEN** 系统在该命名空间创建 Issuer，且不会让其他命名空间的证书表单选中它

#### Scenario: 拒绝无 DNS 前置条件的 AliDNS Issuer
- **WHEN** 用户创建 AliDNS DNS-01 Issuer 但凭据不存在、已停用或 Webhook 未就绪
- **THEN** 系统拒绝创建并返回缺失的前置条件

### Requirement: AliDNS credential protection

系统 SHALL 加密保存 AliDNS AccessKey Secret，并以受控 Kubernetes Secret 供 DNS-01 Solver 使用。

#### Scenario: 创建 AliDNS 凭据
- **WHEN** 用户提交 AccessKey ID 与 AccessKey Secret
- **THEN** 系统加密保存 Secret、在目标命名空间创建包含固定 key 的 Kubernetes Secret，且 API 响应不包含 Secret 值

#### Scenario: 更新凭据但不提交 Secret
- **WHEN** 用户编辑 AliDNS 凭据并将 Secret 输入框留空
- **THEN** 系统保留已加密的 Secret 与 Kubernetes Secret 值

### Requirement: AliDNS webhook installation

系统 SHALL 使用固定版本的受控 K3s HelmChart 安装并检测 AliDNS Webhook。

#### Scenario: 安装 AliDNS Webhook
- **WHEN** cert-manager 已就绪且用户触发 AliDNS Webhook 安装
- **THEN** 系统创建 `cert-manager/cylism-alidns-webhook` HelmChart，使用 `https://wjiec.github.io/alidns-webhook`、`alidns-webhook` 和版本 `1.0.3`

#### Scenario: Webhook 未就绪
- **WHEN** Webhook Deployment 未达到 Available 状态
- **THEN** 系统将 DNS-01 功能标记为不可用并展示 HelmChart 或 Deployment 错误

### Requirement: Certificate operations visibility

系统 SHALL 展示与 Certificate 关联的 CertificateRequest、Order、Challenge 及续期信息。

#### Scenario: DNS-01 Challenge 失败
- **WHEN** Certificate 的关联 Challenge 状态为失败
- **THEN** 系统展示域名、挑战类型、状态和 cert-manager 返回的失败原因

#### Scenario: 查看续期时间
- **WHEN** Ready Certificate 的 status 包含 renewalTime
- **THEN** 系统展示下次续期时间与证书到期时间

### Requirement: Managed domain issuer association

系统 SHALL 为受管域名保存默认 Issuer 名称与类型。

#### Scenario: 使用域名默认签发者
- **WHEN** 应用发布选择已配置默认 ClusterIssuer 的受管域名
- **THEN** 系统将 Issuer 名称和 `ClusterIssuer` 类型一起应用到证书相关资源
