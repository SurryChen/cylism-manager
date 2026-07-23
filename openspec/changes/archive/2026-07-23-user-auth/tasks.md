## 1. 数据模型与配置

- [x] 1.1 新增 User 模型（id, username, password_hash, created_at, updated_at）
- [x] 1.2 更新 Store 层自动迁移 User 表
- [x] 1.3 config.yaml 新增 auth 配置段（admin_user, admin_password, jwt_secret, token ttl）
- [x] 1.4 AuditLog 模型新增 user_id 字段

## 2. JWT 认证服务

- [x] 2.1 实现 JWT 签发（access token + refresh token, HMAC-SHA256）
- [x] 2.2 实现 JWT 验证与解析
- [x] 2.3 实现 bcrypt 密码哈希与验证

## 3. 管理员初始化

- [x] 3.1 Platform 启动时检测 User 表是否为空
- [x] 3.2 若为空则从配置文件读取凭据创建管理员

## 4. 认证 API

- [x] 4.1 实现 POST /api/auth/login
- [x] 4.2 实现 POST /api/auth/refresh
- [x] 4.3 实现 GET /api/auth/me

## 5. 认证中间件

- [x] 5.1 实现 JWT AuthMiddleware（验证 Bearer token，注入 user_id 到 context）
- [x] 5.2 Router 中对 /api/auth/* 之外的所有路由应用中间件
- [x] 5.3 审计中间件从 context 读取 user_id 写入 AuditLog

## 6. 前端登录

- [x] 6.1 新增登录页面（/login 路由）
- [x] 6.2 实现 token 存储（localStorage）和请求拦截（自动附加 Bearer token）
- [x] 6.3 实现路由守卫（未登录跳转登录页、token 过期自动跳转）
- [x] 6.4 App.vue 中添加登出按钮和用户状态显示
