# Design: Certificate Issuer Lifecycle

## Architecture

cert-manager 与 Kubernetes 仍是 Issuer、Certificate、Order、Challenge 的唯一事实来源。SQLite 仅保存 DNS 凭据的加密副本和受控安装配置；每次创建或更新 AliDNS Issuer 时，平台在 `cert-manager` 命名空间 upsert 一个平台命名的 Secret。

```text
Certificates UI
  -> /api/certs/issuers, /api/certs/dns-credentials, /api/certs/operations
  -> Manager API
     -> Kubernetes Issuer / ClusterIssuer / CertificateRequest / Order / Challenge
     -> cert-manager namespace Secret (AliDNS credentials)
     -> K3s HelmChart (AliDNS webhook only)
```

## Issuer Model

`IssuerInfo` 将增加签发模式、ACME Server、邮箱、HTTP Ingress Class、DNS Provider 和最近 Ready 条件时间。创建与更新请求只接受以下模式：

- `self_signed`：用于内部测试。
- `acme_http01`：生成 ACME Solver，Ingress Class 固定默认 `traefik`，可显式覆盖为集群中已安装的 Ingress Class。
- `acme_alidns`：必须引用已启用的 AliDNS 凭据和已就绪的 AliDNS Webhook。

Issuer 可以是命名空间级 `Issuer` 或集群级 `ClusterIssuer`。ClusterIssuer 的 AliDNS Secret 固定写入 `cert-manager` 命名空间；命名空间级 Issuer 的 Secret 写入 Issuer 自身命名空间。

ACME production/staging server 只接受 Let’s Encrypt 官方目录地址，避免平台成为任意外部 ACME 代理。账户私钥 Secret 名称由平台根据 Issuer 名称生成。

## AliDNS Provider

平台使用固定供应链：

- Helm 仓库：`https://wjiec.github.io/alidns-webhook`
- Chart：`alidns-webhook`
- Chart 版本：`1.0.3`
- Release：`cylism-alidns-webhook`
- 安装命名空间：`cert-manager`
- Solver：`groupName = acme.cylism.io`，`solverName = alidns`

创建 DNS-01 Issuer 前，Manager 检查 Webhook HelmChart 和对应 Deployment 是否就绪。安装 API 只创建上述固定的 K3s HelmChart，禁止传入自定义仓库、版本、values 或 groupName。

AliDNS 凭据字段为 AccessKey ID 与 AccessKey Secret。Secret 名称按凭据 ID 生成，内容使用 `access-key-id`、`access-key-secret` 两个固定 key。数据库中的 AccessKey Secret 使用现有 AES 加密；更新时 Secret 留空表示保留原值。

## Operations View

新增读取以下资源并按 Certificate 关联返回：

- `CertificateRequest`：证书请求状态、原因、创建时间。
- `Order`：ACME Order 状态与 URL。
- `Challenge`：DNS/HTTP 挑战类型、域名、状态、原因和呈现状态。

Certificate 列表增加“续期”状态：Ready Certificate 在 `renewalTime` 存在时显示下一次续期时间；未就绪时展示关联运维资源的失败原因。所有资源只读，删除 Certificate 保持现有行为。

## Domain Association

受管域名记录增加 `issuer_kind`，默认 `ClusterIssuer`。域名创建和应用发布在引用默认签发者时必须同时传递名称与类型，防止同名 Issuer 与 ClusterIssuer 被错误选择。

## Security and Risks

- DNS 凭据仅接受写入，所有列表、详情和审计响应均不返回 AccessKey Secret；Kubernetes Secret 读取接口继续仅返回 key 列表。
- AliDNS Webhook 是第三方组件。通过固定 URL、Chart 版本和 release 名称减少供应链漂移；页面明确展示版本、状态和错误。
- HTTP-01 仅在域名公网 DNS 指向 Traefik 且 80 端口可达时成功。平台展示 Challenge 状态，但不伪造网络连通性结果。
- DNS-01 需要用户提供最小权限的 AliDNS AccessKey。平台不验证或存储阿里云主账号密码。

## Alternatives Considered

- 仅让用户手工创建 Secret/YAML：实现快，但无法完成平台化申请、凭据保护与排障闭环。
- 支持任意 Webhook/Helm 配置：灵活但会把高权限集群执行入口暴露给普通表单，拒绝采用。
- 直接调用阿里云 API：需要额外 SDK、DNS 逻辑和续期控制，且与 cert-manager 责任重叠，拒绝采用。
