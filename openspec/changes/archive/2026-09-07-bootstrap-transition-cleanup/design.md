# Design: Bootstrap Transition Cleanup

## Boundary

删除“由 Handler 自行创建依赖”的过渡 API，而不是删除所有方便测试的构造函数。
保留的构造函数必须接收其运行需要的全部 Service、Repository、Adapter 和配置，
并且不得根据 nil 值创建替代依赖。

待删除的类别：

- Application、Agent、Agent Operation、Auth、Cluster DNS、Domain、Runtime 的旧
  宽参数构造函数；
- Platform、Node Registry Mirror、Managed OCI Registry、Registry Proxy 的默认
  构造及 `WithService` fallback；
- Certificate Handler 中会构造 Network Service 的兼容构造；
- System Component Handler 的默认/转发构造，改为单一组合构造。

## Migration

1. 先将每个测试替换为相应严格构造函数，并在测试中显式创建 Fake Service 或
   最小 Adapter。
2. 搜索旧符号的所有调用；删除符号后以编译器确认无遗漏。
3. 对 System Component，保留一个接收 `ComponentService` 和
   `ComponentListService` 的构造函数，Bootstrap 与测试均使用它。
4. 不删除业务层的历史数据/资源兼容逻辑，如 `GetLegacy`、旧 Docker Hub 资源
   重命名和现有 REST endpoint 的兼容响应。

## Verification

- 每一组构造删除后运行对应 API 包测试。
- `rg` 不得再找到被删除构造函数或 Deprecated transition 注释。
- 运行 `go test ./...`、`go build ./...`、前端构建、严格 OpenSpec 校验及
  `git diff --check`。

## Risks

测试以前通过 nil 或完整 `*k8s.Client` 进入 fallback；迁移后必须改用窄 adapter
或显式 nil dependency。保持 Handler 的字段初始化和 HTTP 断言不变，可避免业务
行为变化。编译失败是刻意的迁移信号，而非向后兼容层重新引入的理由。
