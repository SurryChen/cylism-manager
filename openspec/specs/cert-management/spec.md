## Purpose
证书生命周期管理，包括签发、续期、吊销和导入 SSL/TLS 证书，并负责 DNS-01 凭据、Issuer 状态以及 Kubernetes 证书资源之间的一致性。
## Requirements
### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

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

### Requirement: Issuer lifecycle management

系统 SHALL 管理 cert-manager Issuer 与 ClusterIssuer，并支持自签名、ACME HTTP-01 与 ACME AliDNS DNS-01 签发模式。

#### Scenario: 创建 HTTP-01 ClusterIssuer
- **WHEN** 用户提交名称、邮箱和 HTTP Ingress Class 为 `traefik` 的 ACME HTTP-01 ClusterIssuer
- **THEN** 系统创建带有 HTTP-01 solver 的 ClusterIssuer，并返回其实际 Ready 状态与失败原因

#### Scenario: 创建命名空间级 Issuer
- **WHEN** 用户选择 Issuer 类型并提交目标命名空间
- **THEN** 系统在该命名空间创建 Issuer，且不会让其他命名空间的证书表单选中它

#### Scenario: 拒绝无 DNS 前置条件的 AliDNS Issuer
- **WHEN** 用户创建 AliDNS DNS-01 Issuer 但凭据不存在、已停用或 Webhook 未就绪
- **THEN** 系统拒绝创建并返回缺失的前置条件

### Requirement: AliDNS credential protection

系统 SHALL 加密保存 AliDNS AccessKey Secret，并以受控 Kubernetes Secret 供 DNS-01 Solver 使用。

#### Scenario: 创建 AliDNS 凭据
- **WHEN** 用户提交 AccessKey ID 与 AccessKey Secret
- **THEN** 系统加密保存 Secret、在目标命名空间创建包含固定 key 的 Kubernetes Secret，且 API 响应不包含 Secret 值

#### Scenario: 更新凭据但不提交 Secret
- **WHEN** 用户编辑 AliDNS 凭据并将 Secret 输入框留空
- **THEN** 系统保留已加密的 Secret 与 Kubernetes Secret 值

### Requirement: AliDNS webhook installation

系统 SHALL 使用固定版本的受控 K3s HelmChart 安装并检测 AliDNS Webhook。

#### Scenario: 安装 AliDNS Webhook
- **WHEN** cert-manager 已就绪且用户触发 AliDNS Webhook 安装
- **THEN** 系统创建 `cert-manager/cylism-alidns-webhook` HelmChart，使用 `https://wjiec.github.io/alidns-webhook`、`alidns-webhook` 和版本 `1.0.3`

#### Scenario: Webhook 未就绪
- **WHEN** Webhook Deployment 未达到 Available 状态
- **THEN** 系统将 DNS-01 功能标记为不可用并展示 HelmChart 或 Deployment 错误

### Requirement: Certificate operations visibility

系统 SHALL 展示与 Certificate 关联的 CertificateRequest、Order、Challenge 及续期信息。

#### Scenario: DNS-01 Challenge 失败
- **WHEN** Certificate 的关联 Challenge 状态为失败
- **THEN** 系统展示域名、挑战类型、状态和 cert-manager 返回的失败原因

#### Scenario: 查看续期时间
- **WHEN** Ready Certificate 的 status 包含 renewalTime
- **THEN** 系统展示下次续期时间与证书到期时间

### Requirement: Managed domain issuer association

系统 SHALL 为受管域名保存默认 Issuer 名称与类型。

#### Scenario: 使用域名默认签发者
- **WHEN** 应用发布选择已配置默认 ClusterIssuer 的受管域名
- **THEN** 系统将 Issuer 名称和 `ClusterIssuer` 类型一起应用到证书相关资源

### Requirement: 证书页面受扩展状态控制
系统 SHALL 将证书能力建模为扩展感知页面；当 cert-manager 未安装时，页面展示未启用状态与引导，而不是直接暴露宿主机 acme.sh 操作。

#### Scenario: 未安装 cert-manager
- **WHEN** 用户打开 `/certs`
- **THEN** 系统检测不到 cert-manager CRD 或核心控制器
- **AND** 页面展示“未启用证书扩展”状态和安装说明

#### Scenario: 已安装 cert-manager
- **WHEN** 用户打开 `/certs`
- **THEN** 系统检测到 cert-manager CRD 和控制器运行正常
- **AND** 页面展示 `Certificate` 资源列表与详情入口

### Requirement: 证书资源查看
系统 SHALL 在启用 cert-manager 的前提下展示 Kubernetes `Certificate` 资源的列表和详情。

#### Scenario: 查看 Certificate 列表
- **WHEN** cert-manager 已启用且用户请求证书列表
- **THEN** 系统返回每个 `Certificate` 的 name、namespace、secret_name、ready 状态、renewal_time 或到期时间

#### Scenario: 查看 Certificate 详情
- **WHEN** 用户打开某个 `Certificate` 详情
- **THEN** 系统展示 spec、status、条件和关联 Secret 名称
