# Design: DNS Provider Adapters

## Overview

DNS-01 支持由静态注册的 `DNSProviderAdapter` 驱动。Adapter 是后端编译产物，而不是数据库记录或用户配置，因此平台继续只执行审查过的 Kubernetes 资源形状。

```text
DNS credential request
  -> adapter validates required fields
  -> encrypts value map in SQLite
  -> adapter renders controlled Kubernetes Secret

DNS-01 Issuer request
  -> adapter verifies matching credential and webhook readiness
  -> adapter renders cert-manager dns01.webhook solver
```

## Adapter Contract

每个 Adapter 提供：

- stable provider id、中文显示名称和凭据字段定义；字段包含 key、标签、是否 secret、是否 required。
- 从明文 map 到 Kubernetes Secret `data` 的固定映射。
- 固定的 webhook HelmChart 定义、安装 namespace、release 名与 deployment 就绪判断；若 Provider 使用 cert-manager 内置 solver，则该值为空。
- 固定的 cert-manager `dns01` solver 片段，引用受控 Secret 名称和键名。
- 可选的凭据作用域校验，例如命名空间级 Issuer 必须引用同 namespace Secret。

Provider Registry 仅暴露 `List` 与 `Get(id)`；重复 id 会在启动时失败。初始只注册 `alidns`，并保留：

```text
groupName: acme.cylism.io
solverName: alidns
access-key-id / access-key-secret
https://wjiec.github.io/alidns-webhook
alidns-webhook@1.0.3
```

## Credential Storage And Migration

`DNSCredential` 使用 `EncryptedValues`（AES-GCM 加密 JSON map）与 `ConfiguredFields` 瞬态响应字段。AliDNS 专用列不保留；启动迁移会删除未投入使用的旧列。

Kubernetes Secret 命名统一为 `cylism-dns-<provider>-<credential-id>`。Secret 只有 Adapter 定义的 key，标签包括 `cylism.io/dns-provider=<provider>`。

## API

新增或调整为：

```text
GET  /api/certs/dns-providers
GET  /api/certs/dns-credentials
POST /api/certs/dns-credentials
PUT  /api/certs/dns-credentials/:id
DELETE /api/certs/dns-credentials/:id
GET  /api/certs/dns-providers/:provider/status
POST /api/certs/dns-providers/:provider/install
```

凭据请求使用 `provider` 和 `values` map。只接受已注册 provider 的字段；未知字段、缺失必填字段和空 secret 均拒绝。不保留 Provider 专用兼容路由。

Issuer 请求的 DNS 模式为 `acme_dns01` 并携带 `credential_id`。后端根据凭据 Provider 选择 Adapter；不接受 Provider 专用 DNS 模式。

## UI

证书控制台显示 Provider 卡片，每张卡显示安装/就绪状态和安装操作。新增凭据时先选择 Provider，再动态绘制 Adapter 字段；编辑时 secret 字段留空表示保留。签发者表单中的 DNS-01 仅展示其 Provider webhook 已就绪且凭据可用的选项。

## Security

- Adapter 不接受用户定义 Helm 或 solver YAML。
- 所有 secret field 在 API response、日志和审计详情中脱敏。
- 删除凭据前必须检查 Issuer 引用；被引用时拒绝删除。
- Provider 安装保持固定 repo、chart、version 和 values，供应链变更必须通过代码评审。

## Future Tencent Cloud Adapter

腾讯云应作为独立 Adapter 增加：先验证其 cert-manager DNS webhook 的维护状态、Chart 来源、Secret 键名、solver group/名称与最小 CAM 权限，再将这些常量和测试加入 Registry。核心 credential/API/UI/Issuer 流程无需增加 `if provider == ...` 分支。
