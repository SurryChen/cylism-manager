# Tasks

## 1. Kubernetes certificate primitives

- [x] 1.1 为 Issuer CRUD、CertificateRequest、Order、Challenge 和续期信息扩展 k8s client；先补 fake client 单元测试。
- [x] 1.2 为 AliDNS Webhook 创建受控 HelmChart 安装与状态检查；测试固定仓库、版本和失败状态。
- [x] 1.3 扩展 cert-manager 状态检查与部署 RBAC。

## 2. Credential lifecycle

- [x] 2.1 新增加密 DNS credential 模型和存储方法；测试 Secret 不泄露与空值更新保留凭据。
- [x] 2.2 实现 K8s Secret upsert、受控命名和 Issuer 对凭据的引用。
- [x] 2.3 实现 Cert API 的 Issuer、凭据、Webhook 状态和 operations 路由；添加 handler 测试。

## 3. Certificate console

- [x] 3.1 增加 Issuer 管理与 HTTP-01 / AliDNS DNS-01 创建表单，按 Webhook 状态控制 DNS-01 选项。
- [x] 3.2 增加 AliDNS 凭据管理和 Webhook 安装状态界面，不显示 Secret 值。
- [x] 3.3 增加 Certificate 操作下钻页，展示请求、Order、Challenge 和续期时间。
- [x] 3.4 为受管域名增加 Issuer 类型选择与应用发布的关联传递。

## 4. Verification

- [x] 4.1 执行相关 Go 与 Vue 测试，覆盖成功、认证失败、Webhook 未就绪和 DNS-01 Challenge 失败。
- [x] 4.2 执行 `go test ./...`、`go build ./...`、`npm test -- --run`、`npm run build` 和 OpenSpec validate。
