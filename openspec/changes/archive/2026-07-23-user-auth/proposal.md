## 为什么

当前平台无任何认证机制，所有 API 直接暴露。即使单机部署也需要基本的登录保护，防止未授权访问管理面板。一期实现单管理员 JWT 登录，为后续多用户扩展预留数据模型。

## 变更内容

- 新增 User 数据模型（username + bcrypt 密码哈希）
- 新增 JWT 认证体系（access token + refresh token）
- 首次启动自动创建管理员账号（凭据来自配置文件）
- 新增登录 API（POST /api/auth/login）和 Token 刷新（POST /api/auth/refresh）
- 新增认证中间件，保护所有业务 API
- 前端新增登录页面，未登录自动跳转

## 能力

### 新增能力

- `user-auth`: 管理员 JWT 登录认证、Token 刷新、首次启动初始化

### 修改的能力

- `dashboard-audit`: 审计日志中记录操作者（新增 user_id 字段）

## 影响范围

- 新增 `internal/model/user.go` — User 模型
- 新增 `internal/auth/` — JWT 签发/验证 + bcrypt
- 修改 `internal/api/router.go` — 注册 auth 路由 + 全局认证中间件
- 修改 `config/config.yaml` — 新增 auth 配置段
- 修改 `cmd/platform/main.go` — 首次启动初始化管理员
- 修改前端 `web/` — 新增登录页 + token 管理 + 路由守卫
