## Context

前序治理已将应用、监控、基础设施和平台控制面的大部分 REST 调用从视图迁移到 `web/src/api/`，并为依赖路由或筛选条件的读取使用 `useAsyncResource`。剩余页面存在两类不一致：已存在领域 API 的模块只拥有读取而 mutation 留在视图中；Chart 仓库与镜像仓库尚没有完整、明确的 API 所有权。`ProjectEnvironments.vue` 直接发起并串联多个请求，在 `projectID` 改变时只能手动比较 ID，无法取消旧请求。

## Goals / Non-Goals

**Goals:**

- 将该范围内每个业务 endpoint 归入一个命名 API 模块，业务参数与可选 request options 分离。
- 保持既有 HTTP 方法、路径、查询参数、URL 编码、payload 和已解包返回值；未传 options 时保持现有 mock 兼容的调用形态。
- 用页面局部的 `useAsyncResource` 管理项目环境的可重叠读取，在路由变化和卸载时取消或忽略过期结果。
- 让创建、更新、检测、同步、删除失败留在最小操作区域，既不关闭相关 modal，也不清空已成功加载的数据。

**Non-Goals:**

- 不修改后端 API、数据模型、Kubernetes 路由资源、证书签发或项目环境业务规则。
- 不拆分已有页面、引入全局 store/错误总线，或为单个 REST endpoint 新建多层目录。
- 不在本 change 中治理 `RuntimeManagement.vue`、`DBAdmin.vue`、`Login.vue` 或 Chat Drawer。

## Decisions

### 在既有领域模块中扩展而非创建泛化操作层

域名 mutation 归入 `domains.js`，路由 mutation 归入 `sites.js`，项目环境归入 `applications.js`；镜像仓库新增小型 `image-registries.js`，Chart 仓库新增 `chart-repositories.js`。这与现有“一个业务域一个 API 模块”的模式一致，且避免让视图继续拥有 endpoint 细节。

备选方案是建立通用 `operations.js` 或继续把 mutation 留在页面。通用模块会抹平业务边界，后者无法建立路径编码和参数契约的独立测试，因此不采用。

### 项目环境读取以单一页面资源为边界

`ProjectEnvironments.vue` 使用一个 `useAsyncResource` 读取项目、应用、命名空间冲突和当前项目环境。资源 loader 接收 `projectID`，仅在结果仍对应当前路由时提交数据；组件卸载自动取消请求。页面保留自身表单和 mutation 状态，不升级为全局缓存。

备选方案是在现有 `loadProject` 中继续手动比较 `projectID`。这只能避免部分旧结果写入，不能统一处理加载状态与卸载取消，因此不采用。

### 失败按操作表面展示

路由创建/删除、仓库保存/检测/删除、环境保存/同步/删除和域名操作继续使用页面已有的局部错误区域。确认 modal 的失败不关闭确认对象，编辑 modal 的失败不重置输入。`Sites.vue` 不再吞掉异常或仅写入控制台。

## Risks / Trade-offs

- [迁移 wrapper 时改变请求参数或路径编码] → 先添加 API 契约测试，覆盖 path、method、payload、ID 编码与 options。
- [项目切换时旧请求覆盖当前数据] → 通过 deferred request 和 unmount 测试验证 `useAsyncResource` 的最新请求胜出。
- [统一页面级错误使成功数据被遮蔽] → mutation 只更新现有局部错误，不重置列表、表单和确认对象。
- [小型 API 模块增加文件数量] → 仅为独立业务域创建一个扁平模块及其同名测试，避免组件或目录拆分。

## Migration Plan

1. 先为所有新增命名函数编写 API 契约测试。
2. 扩展/新增 API 模块，再逐页迁移调用并补局部失败状态。
3. 为项目环境补充旧响应隔离和卸载取消测试，为 Chart 仓库补齐页面测试。
4. 运行前端、后端、构建和 OpenSpec 全量验证；回归时可回退本次前端 API wrapper 与视图调用改动，无数据迁移。

## Open Questions

- 无。
