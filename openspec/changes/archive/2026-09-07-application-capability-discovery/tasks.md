# Tasks

## 1. Application metadata

- [x] 1.1 先添加 Store/Model 失败测试，覆盖 capability 的 JSON 持久化、历史空值、规范化与原子拒绝无效输入。
- [x] 1.2 实现 Application capability 存储、规范化/校验与替换操作；执行 gofmt 和相关 Store 测试。
- [x] 1.3 添加 Handler 失败测试并实现 `PUT /api/applications/:id/capabilities`；验证错误输入、Application 不存在与响应兼容性。

## 2. Read-only discovery and runtime

- [x] 2.1 添加 fake Kubernetes API 测试，覆盖批量汇总 TCP/UDP Service、LoadBalancer 地址、Deployment/StatefulSet 副本与 Ready Pod 节点。
- [x] 2.2 实现受限 DTO 和 Namespace 批量运行态汇总函数，明确不映射 Secret、模板 Spec 与环境变量。
- [x] 2.3 添加并实现 `GET /api/applications/discovery`、`GET /api/applications/:id/runtime`；验证项目/环境归属、capability 过滤和 Kubernetes 降级。

## 3. Application UI

- [x] 3.1 在应用详情页添加能力标签的行编辑器与保存状态；不为特定 capability 添加专用 UI 分支。
- [x] 3.2 添加前端测试，覆盖空列表、规范化保存结果与 API 错误展示。

## 4. Verification

- [x] 4.1 每项完成后执行 gofmt、相关 Go/前端测试，并完成受影响调用点搜索。
- [x] 4.2 执行 `go test ./...`、`go build ./...`、`npm test -- --run`、`npm run build` 与 `openspec validate application-capability-discovery --strict`。
- [x] 4.3 对每个修改的业务代码文件执行 sec-code 安全扫描并上报风险情况。
