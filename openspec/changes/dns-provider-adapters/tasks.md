# Tasks

## 1. Provider contract

- [x] 1.1 定义 DNSProviderAdapter、Provider Registry、字段描述与固定 webhook 规范；为 AliDNS 注册实现单元测试。
- [x] 1.2 将 AliDNS HelmChart 状态、安装和 Secret 渲染迁移到 Adapter；保留旧 API 兼容别名并测试固定配置。
- [x] 1.3 扩展 Issuer DNS-01 渲染为由 Adapter 驱动，并测试拒绝未知 Provider 和未就绪 Provider。

## 2. Credential migration and API

- [x] 2.1 为通用加密 value map 重构 DNSCredential，删除未投入使用的 AliDNS 专用列并测试脱敏与空值更新。
- [x] 2.2 实现 Provider 列表、通用 credential 和 Provider 状态/安装 API；删除被引用凭据时拒绝。

## 3. Console

- [x] 3.1 证书控制台改为 Provider 状态卡与动态凭据字段表单。
- [x] 3.2 DNS-01 Issuer 表单按已就绪 Provider 和凭据动态选择；保留 HTTP-01 与自签名流程。

## 4. Verification

- [x] 4.1 执行新增 Go/Vue 单元测试，覆盖 AliDNS 迁移、unknown provider、凭据保护和 webhook 未就绪。
- [x] 4.2 执行 `go test ./...`、`go build ./...`、`npm test -- --run`、`npm run build` 与 OpenSpec strict validate。
