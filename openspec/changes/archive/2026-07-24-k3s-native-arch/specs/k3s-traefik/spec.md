## ADDED Requirements

### Requirement: IngressRoute 列表
系统 SHALL 展示集群中所有 Traefik IngressRoute 资源。

#### Scenario: 查看路由列表
- **WHEN** 用户访问站点管理页面
- **THEN** 系统列出所有 IngressRoute，包含域名、匹配规则、后端服务、TLS 状态

### Requirement: IngressRoute 创建
系统 SHALL 支持创建新的 Traefik IngressRoute，配置域名路由到后端 Service。

#### Scenario: 创建 HTTP 路由
- **WHEN** 用户提交域名、后端 Service 名称和端口
- **THEN** 系统创建 IngressRoute CRD，Traefik 自动生效

#### Scenario: 创建 HTTPS 路由（含 TLS）
- **WHEN** 用户提交域名并启用 TLS，且 cert-manager 已签发证书
- **THEN** 系统创建带 tls.secretName 的 IngressRoute

### Requirement: IngressRoute 删除
系统 SHALL 支持删除 IngressRoute 资源。

#### Scenario: 删除路由
- **WHEN** 用户确认删除某 IngressRoute
- **THEN** 系统调用 K8s API 删除对应 CRD
