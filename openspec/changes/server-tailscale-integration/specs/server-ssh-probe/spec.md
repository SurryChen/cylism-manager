## ADDED Requirements

### Requirement: SSH 连通性检测
系统 SHALL 支持对已注册服务器执行 SSH 连通性快检。

#### Scenario: SSH 可达
- **WHEN** 用户对具有有效 SSH 凭据的服务器触发连通性检测
- **THEN** 系统使用存储的 SSH 凭据（密码或私钥）尝试连接，返回 `{ reachable: true, latency_ms: <ms> }`

#### Scenario: SSH 不可达
- **WHEN** SSH 连接超时（5s）或被拒绝
- **THEN** 系统返回 `{ reachable: false, error: "<具体错误>" }`，不修改服务器记录

#### Scenario: 凭据解密失败
- **WHEN** 加密凭据无法解密（密钥不匹配）
- **THEN** 系统返回 500 错误，不执行连接
