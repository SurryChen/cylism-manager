## Why

Kubernetes 资源页的“服务”和“配置”列表为了展示端点数量和引用数量，在首屏同步发起大量 Kubernetes API 调用。集群中的 Service 或命名空间增长后，列表加载时间随资源数量线性增长，影响日常排查效率。

## What Changes

- 为 Service 列表新增轻量查询模式，跳过逐 Service 的端点数量统计。
- 将资源页 Service 列表改为轻量模式，并移除列表中的端点数量展示；端点详情保留按需查询。
- 将 ConfigMap/Secret 列表改为复用轻量元数据查询，跳过工作负载引用扫描，并移除引用数量展示。
- 将 ConfigMap/Secret 的引用关系改为在用户打开详情或执行删除校验时读取。
- 将配置页的命名空间列表改为打开新建表单时按需加载。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `k3s-service-discovery`: Service 列表 API 支持跳过端点数量汇总的轻量查询模式，前端列表不再首屏展示端点数量。
- `k3s-config`: 配置管理列表使用不含工作负载引用的轻量元数据响应，引用关系在详情中展示。
- `frontend-resource-request-lifecycle`: 配置资源页的首屏请求只加载列表所需数据，新建表单依赖的数据延迟至用户发起操作时加载。

## Impact

- 后端：`internal/k8s/service.go`、`internal/api/infrastructure/kubernetes/k8s_handler.go` 及其测试。
- 前端：`web/src/api/kubernetes.js`、`web/src/views/resources/Services.vue`、`web/src/views/resources/Configs.vue` 及其测试。
- API：新增可选 Service 查询参数；现有默认 Service 响应及详情接口保持兼容。
- 不引入新依赖、数据迁移或缓存策略。
