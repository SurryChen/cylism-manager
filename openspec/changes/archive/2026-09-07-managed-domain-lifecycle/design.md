# Design: Managed Domain Certificate Lifecycle

## Domain Boundary

受管域名是可被一个环境命名空间内应用入口使用的精确 Host。它不是 DNS Zone，也不是单纯的字符串白名单。创建完成的域名应有一个由平台管理的 Certificate 和同命名空间 TLS Secret。

```text
DNS credential + ClusterIssuer
        -> managed domain (hostname + namespace + issuer)
        -> Certificate / TLS Secret in namespace
        -> application release selects managed domain
        -> Ingress references the managed TLS Secret
```

`ClusterIssuer` 是跨命名空间可引用的资源。命名空间级 `Issuer` 不适合作为全局域名资产的默认签发者，故本流程不支持它。

## Persistence And Kubernetes Resources

`ManagedDomain` 增加以下持久字段：

- `Namespace`：TLS Secret 与 Certificate 所在命名空间，创建后不可直接修改；迁移前的记录为空，标记为未绑定。
- `CertificateName`：平台生成的 `cylism-domain-<id>`。
- `TLSSecretName`：平台生成的 `cylism-domain-<id>-tls`。
- `IssuerRef` 与 `IssuerKind`：固定引用一个已就绪的 `ClusterIssuer`，其中 Kind 固定为 `ClusterIssuer`。

Kubernetes `Certificate` 是证书状态、到期时间、续期时间和失败原因的唯一事实来源；SQLite 不缓存这些状态。域名创建流程为：先创建数据库资产获得 ID，再创建对应 Certificate。若后者失败，资产保留并显示失败原因，用户可执行重试。

旧的无 Namespace 域名保持可读，但不会被 HTTPS 发布选择；用户必须编辑并绑定命名空间和签发者。平台不自动猜测旧域名应属于哪个环境。

## API

`POST /api/domains` 与 `PUT /api/domains/:id` 接受：

```json
{
  "hostname": "api.example.com",
  "namespace": "production",
  "issuer_ref": "letsencrypt-aliyun-prod",
  "description": "production API",
  "enabled": true
}
```

服务端必须验证：

- Host 是合法精确 DNS 名称，不接收 `*.` 泛域名作为应用入口。
- Namespace 存在。
- `ClusterIssuer` 存在且 Ready。
- 同一 Host 不能被两个受管域名资产占用。

新增：

- `POST /api/domains/:id/certificate`：幂等创建/更新该域名的 Certificate，用于首次申请或失败后的重试。
- `GET /api/domains/:id/operations`：复用 CertificateRequest、Order、Challenge 关联信息。

`GET /api/domains` 返回 Certificate 派生的状态、TLS Secret、续期时间、失败原因与已关联的应用入口计数，但不返回任何 Secret 数据。

## Application Release Integration

发布请求中的 `endpoint.domain_id` 仍引用域名资产。`prepareManagedDomain` 应验证该域名：已启用、绑定命名空间与应用环境一致、TLS 启用时 Certificate 已 Ready。然后将 Host、Issuer 信息和 `TLSSecretName` 写入受控 ReleaseSpec。

渲染公网 HTTPS Ingress 时：

- `Ingress.spec.tls[].secretName` 使用 `ManagedDomain.TLSSecretName`。
- 不渲染新的 Certificate；域名资产已负责其生命周期。

同一个精确 Host 在同一命名空间可被多条不同 Path 路由复用，应用发布必须拒绝与既有入口相同 Host + Path 的冲突。跨命名空间复用同一 Host 被拒绝，避免 TLS Secret 与 Ingress 所属命名空间不一致。

删除域名时，若仍存在引用该 `domain_id` 的应用入口，服务端返回冲突。无引用时删除对应 Certificate；cert-manager 生成的 TLS Secret 按 Certificate 的默认清理行为处理，不读取或暴露其中私钥。

## DNS Semantics

DNS-01 使用 AliDNS webhook 创建临时 `_acme-challenge` TXT 记录，证明域名控制权并完成证书签发。它不会将浏览器流量导向集群。

域名详情应展示“业务解析待配置”与当前集群入口说明，指导用户创建 A/AAAA/CNAME 记录。直接写业务 DNS 记录需要单独的云 DNS Provider API Adapter、幂等记录归属和回滚设计，不能复用 cert-manager webhook 凭据接口，因此不在本变更实现。

## UI

域名页面标题与主按钮改为“受管域名”和“申请 HTTPS 域名”。表单依次选择 Host、环境命名空间、已就绪 ClusterIssuer、说明。提交后直接进入签发状态而非静态域名列表。

域名表格展示 Host、命名空间、签发者、证书状态、TLS Secret、续期时间和关联入口；提供“重试签发”“签发过程”“编辑”“删除”。手工创建 Certificate 仍保留在证书控制台，但标记为独立证书，不自动成为受管域名。

## Security

- 不展示 TLS Secret 内容或私钥。
- 仅接受平台选择的、Ready 的 ClusterIssuer，拒绝自由输入 Issuer 引用。
- 域名删除前检测引用关系，防止仍在提供 HTTPS 时删除证书。
- DNS Provider 凭据继续仅由 cert-manager webhook 用于 ACME Challenge；本变更不扩大其云 API 使用范围。
