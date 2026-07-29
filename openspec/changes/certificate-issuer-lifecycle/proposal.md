# Certificate Issuer Lifecycle

## Why

证书页面目前只能选择已经存在且就绪的 cert-manager Issuer 创建 Certificate。平台无法创建 ACME 签发者、保存 DNS 凭据、安装 AliDNS DNS-01 Solver，用户仍需切换到命令行完成证书申请与排障。

## What Changes

- 新增 Issuer / ClusterIssuer CRUD，支持 ACME HTTP-01、ACME AliDNS DNS-01 和自签名签发者。
- 新增加密保存的 DNS Provider 凭据；平台将凭据写入受控的 Kubernetes Secret，API 永不返回 Secret 值。
- 新增受控的 AliDNS Webhook 安装与状态检查，使用固定的 `wjiec/alidns-webhook` Chart `1.0.3`。
- 为受管域名记录签发者名称和类型，应用发布可使用正确的默认签发者。
- 新增 CertificateRequest、Order、Challenge 的只读运维视图，以及证书续期状态和失败原因。
- 扩展部署 RBAC，使 Manager 能读写上述 cert-manager 资源及受控凭据 Secret。

## Non-goals

- 不提供任意 Helm Chart、任意 Webhook Solver 或任意 YAML 的执行入口。
- 第一版不支持 Cloudflare、DNSPod、Route53 等非 AliDNS Provider。
- 不导入第三方购买证书的 TLS 私钥；仅管理 cert-manager 签发的 Secret。
- 不自动创建阿里云账号或申请 AccessKey。
