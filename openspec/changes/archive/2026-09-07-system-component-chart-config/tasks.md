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

## 5. CoreDNS Static Manifest Support

- [x] 5.1 调查当前 K3s CoreDNS 静态 Deployment 清单，确认 HelmChartConfig 不会影响其滚动策略或调度。
- [x] 5.2 实现 CoreDNS 受控 Deployment 更新、目标节点校验、恢复默认与启动后定期重放，补后端测试。
- [x] 5.3 在系统组件页面显示固定节点，并提供迁移入口、可调度节点选择与 CoreDNS 安全基线，补 Vue 测试。
- [x] 5.4 执行全量验证、安全扫描上报，并等待用户确认后归档变更。

## 6. Runtime Controller Source Adapters

- [x] 6.1 定义 `helm_chart`、`static_deployment`、`embedded`、`unknown` 控制模式，并补运行时探测测试。
- [x] 6.2 实现 HelmChart、Deployment 和 ServiceLB 运行时探测；静态 Deployment 使用通用受控字段适配器。
- [x] 6.3 改造 API 保存、恢复与重放逻辑，保存控制模式快照并拒绝内置或未知组件写入。
- [x] 6.4 在控制台显示控制模式、探测依据和能力，依据能力展示配置、迁移和恢复操作。
- [x] 6.5 执行全量验证、安全扫描上报，并等待用户确认后归档变更。
