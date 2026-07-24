## Why

当前排查问题或验证数据时，需要手动打开 SQLite 文件用 sqlite3 命令行查询，门槛高、效率低。管理员需要一个 Web 界面直接浏览和操作数据库原始数据。

## What Changes

- 新增 `GET /api/admin/tables` 获取所有表名
- 新增 `GET /api/admin/tables/:name` 分页查询表数据（支持排序）
- 新增 `POST /api/admin/tables/:name` 新增记录
- 新增 `PUT /api/admin/tables/:name/:id` 更新记录
- 新增 `DELETE /api/admin/tables/:name/:id` 删除记录
- 后端统一过滤 `json:"-"` 敏感字段，自动排除 `id`、`created_at`、`updated_at`、`deleted_at` 等自动字段
- 前端导航栏新增"数据管理"入口，独立页面展示表选择器、分页数据表格、增删改弹窗

## Capabilities

### 新增能力

- `db-admin`: 数据库管理模块，支持表级别数据浏览与增删改查

### 修改的能力

- `ui-navigation`: 导航栏新增"数据管理"入口

## 影响范围

- 新增 `internal/api/db_admin_handler.go` — 5 个 handler
- 修改 `internal/api/router.go` — 注册路由
- 新增 `web/src/views/DBAdmin.vue` — 前端页面
- 修改 `web/src/App.vue` — 导航栏添加入口
- 修改 `web/src/router/index.js` — 注册路由
