# DNS Provider Adapter Registry

## Why

证书控制台当前虽然保存了 `DNSCredential.Provider`，但 AliDNS 的凭据字段、Kubernetes Secret、cert-manager webhook、HelmChart 和 Issuer DNS-01 渲染均为专用逻辑。第二个 DNS 提供商将导致 handler、k8s client 和前端持续分支膨胀。

## What Changes

- 引入编译期注册的 DNS Provider Adapter，描述凭据字段、Secret 键名、DNS-01 solver、受控 webhook HelmChart 与就绪探测。
- 将 DNS 凭据从 AliDNS 专用 AccessKey 字段迁移为加密的 provider value map；API 只返回字段是否已配置。
- 迁移现有 AliDNS 为第一个 Adapter，并保持现有 Secret 键名和既有凭据可用。
- 以通用 API 和页面展示 Provider 列表、Provider 特定凭据表单与 DNS-01 签发者选择。

## Non-goals

- 本变更不接入腾讯云或其他未经 Chart、solver 和最小权限验证的 DNS Provider。
- 不允许用户提交任意 webhook、Helm 仓库、Chart、Secret 键名或 raw solver YAML。
- 不改变 HTTP-01、自签名 Issuer 和 Certificate 资源作为事实来源的行为。

## Impact

- Affected specs: `cert-management`
- Affected code: DNS credential model/store/API、cert-manager Kubernetes adapter、证书控制台与测试
