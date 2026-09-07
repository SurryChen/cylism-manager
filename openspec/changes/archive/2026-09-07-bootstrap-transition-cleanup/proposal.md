# Bootstrap Transition Cleanup

## Why

阶段三至五已将生产依赖组装迁移到 `bootstrap.Container`，但 API 包仍导出会
创建 Service、适配具体 Kubernetes Client 或填充默认 Registry 的过渡构造函数。
它们只由仓库测试使用，会让新的调用方再次绕过 Composition Root，并掩盖依赖
遗漏。

## What Changes

- 删除所有阶段性弃用的 Handler 构造函数及其 fallback Service/Adapter 创建。
- 将受影响测试改为使用严格的显式依赖注入构造函数。
- 将 System Component Handler 收敛为一个直接接收已组装 Service 的构造入口，
  删除只作参数转发或重建 Service 的临时包装。
- 删除没有生产调用方的临时 Bootstrap 包装，并通过全仓库搜索证明没有遗留调用。

## Non-goals

- 不删除 REST 路径、数据库迁移或历史 Registry Proxy 资源迁移逻辑。
- 不改变请求拥有的异步任务、WebSocket 或后台任务生命周期。
- 不改变仍作为正式窄依赖注入 API 的 `WithDependencies`、`WithAdapter` 和
  `WithKubernetesAdapter` 构造函数。

## Impact

- 受影响包：`internal/api/{agent,application,auth,delivery,infrastructure,runtime,system}`
  与其测试，以及 `internal/bootstrap`。
- `internal/...` 仅能在本模块树内导入；清理会使仓库内部调用必须显式提供所需
  Service/Adapter，但不改变对外 HTTP 契约。
- 阶段五的 `background-lifecycle-composition` change 保持未归档状态，仍需在其
  验证确认后单独归档。
