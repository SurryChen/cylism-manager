# grpc-security Specification

## Purpose
TBD - created by archiving change grpc-mtls-keepalive. Update Purpose after archive.
## Requirements
### Requirement: 平台 CA 管理
系统 SHALL 在启动时检查 data/tls/ 目录下的 CA 证书和私钥，若不存在则自动生成自签名 CA（ECDSA P256）。

#### Scenario: 首次启动生成 CA
- **WHEN** 平台首次启动且 data/tls/ca-cert.pem 不存在
- **THEN** 系统生成一对 ECDSA P256 CA 证书和私钥，有效期 10 年，保存到 data/tls/

#### Scenario: 已有 CA 直接加载
- **WHEN** 平台启动时 data/tls/ca-cert.pem 已存在
- **THEN** 系统直接加载已有 CA，不重新生成

### Requirement: Agent 证书签发
系统 SHALL 在部署 Agent 时，由平台 CA 为该 Agent 签发客户端证书。

#### Scenario: 部署时签发证书
- **WHEN** 用户对服务器触发部署
- **THEN** 系统以 CA 签发该 Agent 的证书（CN=agent-{server_id}），保存到 data/tls/servers/{id}-cert.pem 和 {id}-key.pem，并通过 SSH 上传到远端

### Requirement: mTLS 双向认证
系统 SHALL 在 gRPC 通信中启用 mTLS，平台和 Agent 互相验证对方证书。

#### Scenario: 平台验证 Agent 证书
- **WHEN** Agent 发起 gRPC 连接
- **THEN** 平台验证 Agent 证书是否由平台 CA 签发，验证通过后建立连接

#### Scenario: Agent 验证平台证书
- **WHEN** Agent 作为 gRPC Server 接受连接
- **THEN** Agent 验证平台客户端证书是否由平台 CA 签发，验证通过后接受请求

