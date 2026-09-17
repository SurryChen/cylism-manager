## Why

自托管制品库页面目前只展示部署配置、PVC 和健康状态，用户无法确认 Registry 中实际有哪些仓库、Tag 和可拉取镜像，也无法在控制台清理不再使用的制品。受管 Registry 已保存认证信息且项目已具备 OCI 客户端依赖，现在可以在不暴露凭据的前提下补齐制品可见性和受控删除能力。

## What Changes

- 在受管 Registry 页面采用“自托管制品库 + 概览 / 镜像”标题和页签结构；概览保留运行与配置管理，镜像页提供仓库和 Tag 浏览、搜索、刷新及复制拉取地址。
- 新增受认证的服务端 Registry catalog API，用保存的受管凭据查询 OCI Distribution catalog、Tag 和 manifest 元数据；浏览器不接触 Registry 拉取密码。
- 新增 Tag 与仓库删除工作流：在执行 Registry 的 manifest 删除前，显示同 digest 关联 Tag、平台发布和上线模板引用；有引用时阻止删除，无引用时要求明确确认并审计操作。
- 为不可用 Registry、空目录、分页、远端请求失败及删除后的刷新提供明确状态。

## Capabilities

### New Capabilities

- `managed-oci-registry-catalog`: 浏览并安全管理平台受管 OCI Registry 中的仓库、Tag 和 manifest。

### Modified Capabilities

- None.

## Impact

- 影响 `ManagedOCIRegistryHandler`、delivery routes、Registry service 和应用/发布查询边界。
- 新增受管 Registry 的只读目录、Tag 明细与删除 REST 接口，以及对应 Go 测试。
- 影响 `ManagedOCIRegistries.vue`、前端 API 模块和视图测试。
- 复用现有 `go-containerregistry` 与加密凭据，不新增数据库表或第三方依赖。
