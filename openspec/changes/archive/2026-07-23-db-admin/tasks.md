## 实现任务

### 1. 后端：DB Admin Handler + 路由
- [x] 新增 `internal/api/db_admin_handler.go`，包含 5 个 handler（ListTables, ListRecords, CreateRecord, UpdateRecord, DeleteRecord）
- [x] 在 `internal/api/router.go` 中注册 `/api/admin/tables` 路由组
- [x] 编写 handler 单元测试

### 2. 前端：DBAdmin 页面
- [x] 新增 `web/src/views/DBAdmin.vue`，包含表选择 tabs、数据表格、分页、排序
- [x] 新增/编辑弹窗（动态表单）
- [x] 删除二次确认弹窗

### 3. 前端：路由 + 导航入口
- [x] `web/src/router/index.js` 注册 `/db-admin` 路由
- [x] `web/src/App.vue` 导航栏新增"数据管理"链接

### 4. 全量验证
- [x] `go test -v -count=1 ./...` 全部通过
- [x] `go build ./...` 编译通过
- [x] `cd web && npm run build` 前端构建通过
