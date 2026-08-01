# Design: 应用栈编排

## Ownership

`Application` 是一个可独立部署的服务，只拥有自己的上线模板、Release、Endpoint、ConfigMap/Secret 和 PVC 挂载。

`ApplicationStackTemplate` 归属 `Environment`，以 JSON 保存有序的组件定义。每个组件定义包含唯一的 Application Name、描述、可选入口标记和标准 `ReleaseSpec`。保存模板不创建 Kubernetes 资源。

`ApplicationStackRelease` 归属 Stack Template 与 Environment，保存不可变的解析快照及每个组件对应 Application 和 Release 的结果。组件应用通过 `StackID` 关联到所属栈，仍可作为普通应用打开详情、单独发布或绑定域名。

## Lifecycle

创建或发布栈时，平台在同一 Environment 中按组件名称查找 Application：

1. 不存在时创建一个受栈管理的 Application。
2. 已存在但属于其他栈时拒绝，避免接管普通应用。
3. 已属于当前栈时复用。
4. 为每个组件生成或更新一个命名的单应用上线模板，并调用现有单应用 Release 流程。

组件间通过 Kubernetes Service DNS 通讯；模板中的配置可以使用明确的服务名。栈不再生成一个复合 Deployment，也不持有对外域名。入口组件创建后，用户在该 Application 详情页独立绑定受管域名。

Karakeep 是平台返回的内置模板预设，包含 `karakeep`、`karakeep-meilisearch`、`karakeep-chrome` 三个应用定义。其 PVC、Secret 与内部服务地址由预设表单填写后实例化为普通栈模板。

## API

- `GET/POST /api/application-stacks?environment_id=`：列出或创建环境应用栈模板。
- `GET/PUT/DELETE /api/application-stacks/:id`：读取、修改、删除栈模板。
- `GET/POST /api/application-stacks/:id/releases`：查看或创建栈发布。
- `POST /api/application-stacks/presets/karakeep`：将 Karakeep 表单实例化为环境栈模板。

栈发布请求可提供 `{ "versions": { "karakeep": "..." } }`。未指定组件版本时使用其模板默认版本；任何组件失败时整个栈发布标记失败，但已完成的独立 Application Release 保留真实状态。

## Validation

- 环境、组件名称、镜像、资源、PVC、Secret 与单应用 `ReleaseSpec` 使用同一验证。
- 所有组件名称在当前 Environment 内唯一，并符合 DNS-1123 label。
- 模板只能引用所属 Environment 的 PVC 与镜像仓库授权。
- 栈中最多一个入口组件；入口标记只用于 UI 提示，不自动创建 Endpoint。
- 删除模板时存在发布记录则拒绝；删除栈不会删除其已创建的应用或 PVC。

## UI

工作台新增“应用栈”区域，展示当前项目环境的栈、组件数量、最近发布状态和入口应用。入口支持“新建应用栈”和“使用 Karakeep 预设”。应用详情移除栈组件区，仅保留单服务模板、发布和域名管理。

通用栈编辑器支持增加/删除组件、填写应用名和基础 ReleaseSpec；首期复用单应用表单中的关键字段。Karakeep 预设只是向同一编辑器填充组件定义，不保留专用数据模型。
