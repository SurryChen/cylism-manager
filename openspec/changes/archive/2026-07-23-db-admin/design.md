## 背景

当前项目有 7 张表（servers、sites、certs、audit_logs、operation_logs、users），排查数据问题需手动 sqlite3 命令行查询，效率低。需要一个 Web 端数据管理界面。

## 目标 / 非目标

**目标：**
- 7 张表全部支持浏览、分页、排序
- 支持动态增删改（表单字段从 GORM 模型反射生成）
- 自动隐藏 `json:"-"` 敏感字段
- 删除二次确认
- 导航栏独立入口

**非目标：**
- 表结构修改（DDL）
- 全文搜索
- 数据导出
- 关系 / 外键可视化

## 技术决策

### 1. 通用表操作 API（反射方式）

用 GORM 原生能力反射表结构，无需为每张表单独写 handler。

```go
// 获取所有表：从 models.go 的 AutoMigrate 列表收集
GET /api/admin/tables → ["servers","sites","certs","audit_logs","operation_logs","users"]

// 分页查询
GET /api/admin/tables/servers?page=1&size=20&sort=id&order=desc
```

通过 GORM 的 `Migrator().HasColumn()` 和反射 struct 来动态获取字段信息。

- 备选：每表独立 handler — 代码膨胀，新增表需同步修改

### 2. 敏感字段过滤

后端写一个通用函数 `filterFields(struct)`：遍历 `json:"-"` tag，将其从 map 中移除后再返回 JSON。

敏感字段清单：
- `users.password_hash` (json:"-")

对于 `servers` 表，`ssh_password`、`ssh_key`、`ssh_key_passphrase` 是 `json:"-"`，在 JSON 序列化时已被 GORM 排除。

### 3. 动态表单生成

前端根据后端返回的 `columns` 元数据（字段名 + 类型）动态渲染表单：
- string → `<input type="text">`
- int/uint → `<input type="number">`
- bool → `<select>` 或 checkbox
- text → `<textarea>`

自动排除：`id`, `created_at`, `updated_at`, `deleted_at`

### 4. 表清单维护方式

在 `db_admin_handler.go` 中硬编码一个表名到 model 的映射：

```go
var tableRegistry = map[string]interface{}{
    "servers":        model.Server{},
    "sites":          model.Site{},
    "certs":          model.Cert{},
    "audit_logs":     model.AuditLog{},
    "operation_logs": model.OperationLog{},
    "users":          model.User{},
}
```

- 备选：`SELECT name FROM sqlite_master` — 会暴露 GORM 内部表，不如显式注册安全

### 5. 前端数据表格

```
[表选择 tabs: servers | sites | certs | audit_logs | operation_logs | users]

[新增按钮]

| col1 ↑↓ | col2 ↑↓ | ... | 操作 |
|----------|---------|-----|------|
| 值       | 值      |     | 编辑 删除 |

[分页: < 1 2 3 >]
```

## 风险与权衡

- **[误删数据]** 删除不可逆 → 二次确认弹窗 + 只有管理员可访问
- **[大表性能]** audit_logs / operation_logs 可能数据量大 → 强制分页（默认 20 条/页）
- **[类型转换]** 反射读写的值类型与表单字符串之间需处理 → 用 GORM 的 Scan/Updates 自动转换
