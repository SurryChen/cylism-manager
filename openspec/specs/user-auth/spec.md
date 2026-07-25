## Purpose
用户认证、JWT Token 管理和管理员初始化。
## Requirements
### Requirement: 管理员登录
系统 SHALL提供登录接口，验证用户名和密码后返回 JWT access token 和 refresh token。

#### Scenario: 登录成功
- **WHEN** 用户以正确的管理员用户名和密码调用 POST /api/auth/login
- **THEN** 系统返回 access_token、refresh_token 和用户信息，HTTP 200

#### Scenario: 密码错误
- **WHEN** 用户以错误的密码调用 POST /api/auth/login
- **THEN** 系统返回认证失败错误，HTTP 401

#### Scenario: 用户不存在
- **WHEN** 用户以不存在的用户名调用 POST /api/auth/login
- **THEN** 系统返回认证失败错误，HTTP 401

### Requirement: Token 刷新
系统 SHALL提供 token 刷新接口，使用有效的 refresh token 换取新的 access token。

#### Scenario: 刷新成功
- **WHEN** 用户以有效的 refresh token 调用 POST /api/auth/refresh
- **THEN** 系统返回新的 access_token 和 refresh_token，HTTP 200

#### Scenario: refresh token 无效或过期
- **WHEN** 用户以无效/过期的 refresh token 调用 POST /api/auth/refresh
- **THEN** 系统返回认证失败错误，HTTP 401

### Requirement: 认证中间件保护 API
系统 SHALL对所有非 `/api/auth/*` 的 API 端点进行 JWT 认证校验。

#### Scenario: 有效 token 访问业务 API
- **WHEN** 用户在请求头中携带有效的 Bearer token 访问 /api/servers
- **THEN** 系统正常处理请求，返回数据

#### Scenario: 无 token 访问业务 API
- **WHEN** 用户未携带 token 访问 /api/servers
- **THEN** 系统返回未授权错误，HTTP 401

#### Scenario: 过期 token 访问业务 API
- **WHEN** 用户携带过期的 access token 访问 /api/servers
- **THEN** 系统返回未授权错误，HTTP 401

### Requirement: 首次启动管理员初始化
系统 SHALL在首次启动（User 表为空）时自动从配置文件创建管理员账号。

#### Scenario: 首次启动创建管理员
- **WHEN** 平台启动且 User 表为空
- **THEN** 系统读取配置文件中的 admin_user 和 admin_password，使用 bcrypt 哈希密码后创建管理员记录

#### Scenario: 再次启动跳过初始化
- **WHEN** 平台启动且 User 表已有记录
- **THEN** 系统跳过管理员初始化，不修改已有用户

### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

