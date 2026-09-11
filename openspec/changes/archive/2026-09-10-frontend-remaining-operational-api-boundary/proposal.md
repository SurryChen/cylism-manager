## Why

前端大多数高频运维页面已经具备领域 API 模块与受控读取生命周期，但域名、路由、镜像与 Chart 仓库、项目环境仍在页面中拼接 REST 路径。尤其是路由操作会静默吞掉失败，项目环境在路由快速切换时没有取消旧请求的能力，增加了运维操作失败无反馈和数据显示错位的风险。

## What Changes

- 扩展域名、路由、应用项目、镜像仓库领域 API 模块，使页面使用命名函数执行既有读写操作。
- 新增 Chart 仓库领域 API 模块，覆盖列表、创建、更新、检测和删除。
- 将 `Domains.vue`、`Sites.vue`、`ImageRegistries.vue`、`ChartRepositories.vue`、`ProjectEnvironments.vue` 迁移到领域 API 函数。
- 为项目环境的项目切换读取接入 `useAsyncResource`，防止旧响应覆盖当前项目。
- 将路由、仓库和项目环境操作失败显示在所属页面区域，保留表单、确认框和已加载数据以便重试。
- 为新增 API 契约、路由切换、局部失败和 Chart 仓库补充回归测试。

## Capabilities

### New Capabilities

- `frontend-remaining-operational-api-boundary`: 剩余运维页面的领域 API 所有权、项目切换读取隔离与局部失败反馈。

### Modified Capabilities

- 无。

## Impact

- 影响 `web/src/api/domains.js`、`sites.js`、`applications.js`，并新增镜像仓库和 Chart 仓库 API 模块及测试。
- 影响域名、路由、镜像仓库、Chart 仓库和项目环境页面及其测试。
- 不修改 Go 后端 endpoint、HTTP 方法、认证、业务 payload、路由资源语义或项目环境规则。
