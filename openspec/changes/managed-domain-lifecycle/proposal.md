# Managed Domain Certificate Lifecycle

## Why

当前“域名”仅保存 Host 与一段可手工输入的 Issuer 文本。创建它不会申请证书、不会生成 TLS Secret，也不会约束应用发布使用哪个命名空间的证书。因此用户无法理解域名资产的作用，且应用发布会再次创建独立 Certificate，容易产生重复签发和不可追踪的入口关系。

## What Changes

- 将受管域名定义为一个绑定到目标命名空间的 HTTPS 域名资产。
- 创建或更新域名时只能选择已就绪的 `ClusterIssuer`；平台自动创建并维护该命名空间中的 cert-manager `Certificate` 与 TLS Secret。
- 域名列表展示证书状态、TLS Secret、续期时间、签发失败原因与关联应用入口；提供重试签发和签发过程下钻。
- 应用发布选择受管域名并启用 HTTPS 时，只引用该域名已管理的 TLS Secret，不再额外创建同一域名的 Certificate。
- 约束应用只能选择与其环境命名空间匹配、证书已就绪的受管域名；删除域名前阻断仍被应用入口引用的域名。
- 将业务流量 DNS 解析与 ACME DNS-01 TXT 验证明确区分：本变更展示人工需要配置的入口解析目标，但不通过 DNS-01 webhook 直接修改 A/AAAA/CNAME 记录。

## Non-goals

- 不直接调用阿里云、腾讯云等云 DNS API 创建业务 A/AAAA/CNAME 记录；DNS-01 webhook 仅用于 cert-manager 的 ACME TXT 验证。
- 不支持将同一个精确 Host 路由到不同命名空间的多个应用。
- 不导入或管理用户在平台外创建的 TLS 私钥 Secret。
- 不改变 cert-manager 对续期和 ACME Challenge 的事实来源。
