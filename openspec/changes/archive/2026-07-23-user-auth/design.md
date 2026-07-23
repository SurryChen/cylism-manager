## 背景

平台当前无认证，所有 API 开放访问。单管理员模式是最小可行方案：一个内置账号 + JWT，无需注册、无需用户管理 UI。

## 目标 / 非目标

**目标：**
- JWT 登录认证保护所有业务 API
- 首次启动自动创建管理员
- 前端登录页 + 路由守卫

**非目标：**
- 多用户注册/管理
- OAuth / SSO
- 角色权限（RBAC）
- 密码修改/重置页面（一期通过配置文件修改）

## 技术决策

### 1. JWT 方案：access + refresh token

- Access token: 2 小时有效期，HMAC-SHA256 签名
- Refresh token: 7 天有效期，存储在客户端（localStorage）
- Access token 过期后用 refresh token 换取新 access token
- 备选：单一长有效期的 token — 安全性差，泄露后无法撤销

### 2. 密码存储：bcrypt

- Go 标准库 `golang.org/x/crypto/bcrypt`，cost=12
- 备选： argon2 — 更安全但需要额外依赖，bcrypt 对此场景足够

### 3. 管理员初始化

- Platform 启动时检查 User 表是否为空
- 若为空，读取配置文件中 `auth.admin_user` / `auth.admin_password`
- 创建用户，密码 bcrypt 哈希后存储
- 明文密码建议初始化后从配置文件中删除

### 4. 中间件注入

- AuthMiddleware 验证 Bearer token
- 将 user_id 注入 gin.Context
- 审计日志中间件读取 user_id 记录操作者

## 风险与权衡

- **[JWT 密钥泄露]** 配置文件中的 `jwt_secret` 泄露可伪造 token → 部署时务必更换随机密钥
- **[Token 存储]** 前端使用 localStorage 存 token → XSS 风险，但单机管理面板场景可接受
- **[首次启动密码]** 配置文件中有明文密码 → 提示用户初始化后删除
