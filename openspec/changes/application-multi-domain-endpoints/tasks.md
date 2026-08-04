# Tasks

## 1. Endpoint Persistence And API

- [x] 1.1 为入口列表、按 ID 查询/更新/删除和精确路由冲突检查添加失败的 Store 测试。
- [x] 1.2 实现入口列表 Store API，保留已有数据兼容性并移除单例替换调用。
- [x] 1.3 将应用入口 HTTP API 改为列表、创建、编辑和单项解绑，并覆盖所有权、域名状态与冲突测试。

## 2. Kubernetes And Release Synchronization

- [x] 2.1 为多个 Host、多个 TLS Secret、相同 Secret 合并和最后入口删除添加 Ingress 渲染测试。
- [x] 2.2 实现从入口列表同步受管 Ingress 的 Kubernetes Applier API。
- [x] 2.3 更新发布、重试和回滚流程，使其不再以 Release 单入口覆盖当前 Ingress，并补充回归测试。

## 3. Console

- [x] 3.1 将应用详情入口区改为列表，支持新增、编辑和单项解绑。
- [x] 3.2 更新应用工作台的公开链接展示，显示主入口及额外入口数。
- [x] 3.3 添加 Vue 测试，覆盖多个域名渲染、创建、编辑、解绑和请求载荷。

## 4. Verification

- [ ] 4.1 每项完成后执行受影响的 Go/Vue 测试，并对修改的业务代码运行安全扫描上报。
- [x] 4.2 执行 `go test ./...`、`go build ./...`、`npm --prefix web test`、`npm --prefix web run build`、`openspec validate application-multi-domain-endpoints --strict` 和 `git diff --check`。
