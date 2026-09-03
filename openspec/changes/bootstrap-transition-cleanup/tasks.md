# Tasks

## 1. Test-first migration

- [x] 1.1 列出所有弃用构造及生产/测试调用点，为每组测试先改为显式依赖注入。
- [x] 1.2 将 Application、Agent、Agent Operation、Auth、Runtime、Cluster DNS 和
  Domain 测试迁移至严格构造函数。
- [x] 1.3 将 Platform、Mirror、Managed Registry、Registry Proxy 和 Certificate
  测试迁移至严格构造函数。
- [x] 1.4 将 System Component Handler 测试迁移至直接注入的组合构造函数。

## 2. Remove transitions

- [x] 2.1 删除上述 Handler 的弃用构造和 fallback Service/Adapter/Registry 创建。
- [x] 2.2 收敛 System Component Handler 的临时包装，保持 Bootstrap 单例实例。
- [x] 2.3 搜索并删除无调用方的 Bootstrap 迁移包装；保留正式窄依赖注入构造和
  业务历史兼容逻辑。

## 3. Verification

- [x] 3.1 每组删除后执行旧符号搜索、相关包测试与 `git diff --check`。
- [x] 3.2 执行宿主机 `go test ./...`、`go build ./...` 和 `npm --prefix web run build`。
- [ ] 3.3 执行 `openspec validate bootstrap-transition-cleanup --strict`，呈报结果并
  等待用户确认后归档。
