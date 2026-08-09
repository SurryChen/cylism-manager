# Tasks

## 1. Backend Persistence

- [x] 1.1 新增 `SystemComponentConfig` 模型并注册 AutoMigrate，补 Store CRUD 测试。
- [x] 1.2 实现 Store CRUD（列表/按 chart 查询/upsert/删除）。
- [x] 1.3 实现 HelmChartConfig CRD 的 apply/get/delete（dynamic client），补 fake dynamic 测试。

## 2. API

- [x] 2.1 实现列表 handler（白名单 + 期望 vs 实际 Deployment 状态）。
- [x] 2.2 实现保存 handler（白名单 + YAML 校验 + apply + 落库）。
- [x] 2.3 实现恢复默认 handler。
- [x] 2.4 注册路由并补 handler 测试。

## 3. Console

- [x] 3.1 实现 `SystemComponents.vue`（列表、编辑 YAML、一键安全基线、恢复默认）。
- [x] 3.2 接入 `ClusterHub` tab 与路由别名。
- [x] 3.3 补 Vue 测试。

## 4. Verification

- [x] 4.1 运行受影响的 Go/Vue 测试。
- [x] 4.2 执行 `go test ./...`、`go build ./...`、`npm --prefix web test`、`npm --prefix web run build`、`openspec validate system-component-chart-config --strict`、`git diff --check`。
