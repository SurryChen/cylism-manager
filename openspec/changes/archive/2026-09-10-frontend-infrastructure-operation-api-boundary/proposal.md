## Why

`Servers.vue`、`Cluster.vue` 与 `PersistentVolumes.vue` 已分别具备部分领域读取 API，但服务器管理、节点维护和持久卷变更仍在页面中直接拼接 REST 路径。这些操作包含驱逐、数据恢复、迁移和删除等高影响行为，当前错误反馈不一致，部分服务器操作甚至只写入浏览器控制台。

## What Changes

- 扩展 `servers.js`、`cluster.js` 与 `storage.js`，使三个页面的读取和变更均通过命名领域 API 函数发起。
- 保持既有 HTTP 方法、路径、参数、payload 与解包返回值不变，并让请求选项与业务参数分离。
- 为节点预检、标签、移除检查，以及 PVC 备份和导入记录等目标相关读取提供可取消的本地生命周期管理。
- 将服务器、集群和存储卷变更失败显示在发起操作的表单、确认框或弹窗中；失败不得清除无关的已加载数据或关闭可重试的操作表面。
- 为 API 契约、弹窗请求竞态、卸载清理、最近成功数据保留及关键变更失败补充回归测试。

## Capabilities

### New Capabilities

- `frontend-infrastructure-operation-api-boundary`: 服务器、集群节点和持久卷操作的前端 API 归属、请求生命周期及局部失败处理。

### Modified Capabilities

- 无。

## Non-goals

- 不修改后端 REST 路由、认证、响应格式、节点维护规则或存储卷工作流。
- 不改变驱逐、迁移、备份、恢复、导入和删除的用户确认语义。
- 不拆分 `Servers.vue`、`Cluster.vue` 或 `PersistentVolumes.vue`，不引入全局状态库或新的 UI 组件体系。
- 不迁移平台设置、运行时聊天、网络站点或其他页面的直接 API 调用。

## Impact

- 受影响前端：`web/src/api/servers.js`、`cluster.js`、`storage.js`，三个对应页面及其 API/组件测试。
- 现有页面使用的 HTTP 合同保持兼容；没有新增依赖或后端数据迁移。
