## Purpose
证书生命周期管理，包括签发、续期和吊销 SSL/TLS 证书。
## Requirements
### Requirement: 证书签发
系统 SHALL通过 acme.sh 为指定站点签发 SSL 证书，支持 HTTP（webroot）和 DNS 两种验证方式。

#### Scenario: 通过 HTTP 验证签发证书
- **WHEN** 用户对验证方式为 "http" 且具备有效 root_path 的站点触发证书签发
- **THEN** 系统通过 Agent 调用 acme.sh --issue 并传入正确的 webroot 参数，将证书路径存入 Cert 记录，返回证书信息

#### Scenario: 通过 DNS 验证签发证书
- **WHEN** 用户以验证方式 "dns" 触发证书签发
- **THEN** 系统通过 Agent 调用 acme.sh --issue --dns，并将生成的证书路径存储

#### Scenario: 证书签发失败
- **WHEN** acme.sh 签发失败（如 DNS 未传播、webroot 不可访问）
- **THEN** 系统返回 acme.sh 输出的错误详情，不创建 Cert 记录

### Requirement: 证书续期
系统 SHALL支持手动证书续期，以及自动定时续期将在 30 天内到期的证书。

#### Scenario: 手动续期
- **WHEN** 用户对已有证书触发续期
- **THEN** 系统通过 Agent 调用 acme.sh --renew，更新到期时间戳，并在关联服务器上触发 NGINX reload

#### Scenario: 自动续期扫描
- **WHEN** 后台调度器运行并发现证书将在 30 天内到期
- **THEN** 系统自动触发续期并更新证书记录

#### Scenario: 无需续期
- **WHEN** 证书不在续期窗口内
- **THEN** 系统跳过续期，不修改证书

### Requirement: 证书吊销
系统 SHALL允许用户通过 acme.sh 吊销证书并清理关联配置。

#### Scenario: 吊销证书
- **WHEN** 用户对已签发的证书触发吊销
- **THEN** 系统通过 Agent 调用 acme.sh --revoke，将证书状态设为 "revoked"，并从关联的 NGINX 配置中移除 SSL 引用

### Requirement: 证书到期仪表盘
系统 SHALL在仪表盘中包含证书到期信息，高亮显示将在 30 天内到期的证书。

#### Scenario: 仪表盘显示即将到期的证书
- **WHEN** 用户查看仪表盘
- **THEN** 系统返回按到期时间排序的证书列表，即将在 30 天内到期的标记为警告状态

### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

