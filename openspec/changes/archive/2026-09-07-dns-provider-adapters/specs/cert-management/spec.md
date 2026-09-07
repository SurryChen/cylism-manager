# Certificate Management Delta

## ADDED Requirements

### Requirement: Controlled DNS provider adapter registry

系统 SHALL 仅通过编译期注册的 DNS Provider Adapter 支持 DNS-01 提供商，并暴露可用 Provider 及其就绪状态。

#### Scenario: 列出可用 DNS Provider
- **WHEN** 用户打开证书控制台
- **THEN** 系统返回所有已注册 Provider 的名称、凭据字段定义与 webhook 就绪状态，且不返回任何凭据值

#### Scenario: 拒绝未注册 Provider
- **WHEN** 用户为 DNS 凭据或 DNS-01 Issuer 提交未知 Provider
- **THEN** 系统拒绝请求且不创建 Kubernetes Secret、HelmChart 或 Issuer

### Requirement: Provider-agnostic credential lifecycle

系统 SHALL 按 Adapter 定义加密保存 Provider 值并创建受控 Kubernetes Secret。

#### Scenario: Secret 字段留空更新
- **WHEN** 用户更新已存在凭据并省略 secret 字段
- **THEN** 系统保留已有加密值和 Kubernetes Secret 对应值

#### Scenario: 删除被引用凭据
- **WHEN** 用户尝试删除被 DNS-01 Issuer 引用的凭据
- **THEN** 系统拒绝删除并说明引用该凭据的 Issuer

### Requirement: Adapter-rendered DNS-01 Issuer

系统 SHALL 根据 DNS 凭据所属 Provider 渲染对应的 cert-manager DNS-01 solver，而不接受原始 solver YAML。

#### Scenario: 创建 AliDNS DNS-01 Issuer
- **WHEN** 用户选择已就绪的 AliDNS Provider 与启用凭据创建 DNS-01 Issuer
- **THEN** 系统使用 AliDNS Adapter 的固定 webhook 配置、Secret 名称和键名创建 Issuer

#### Scenario: Provider webhook 未就绪
- **WHEN** DNS-01 Issuer 所属 Provider 的 webhook 未就绪
- **THEN** 系统拒绝创建 Issuer，并返回该 Provider 的安装或部署状态
