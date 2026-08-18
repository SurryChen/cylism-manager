## Why

应用模板已经能够将已有 ConfigMap 和 Secret key 投影为只读文件，但配置页面只有查看能力，模板也只能要求用户预先通过 kubectl 创建资源。TLS 证书虽可由受管域名签发，却不能在模板配置中被明确选择、追踪和校验。这使需要配置文件和 TLS 的服务无法在一个连贯流程中上线。

## What Changes

- 为同一应用环境内的 ConfigMap 与 Opaque Secret 提供创建、编辑和删除能力，并保留引用关系可见性。
- 将模板文件挂载中的资源名称输入替换为同命名空间资源选择器，提供快捷创建和跳转资源管理入口。
- 在模板中提供通用 TLS 证书选择/申请辅助操作；它复用受管域名和 cert-manager，不保存私钥或实现应用特定证书逻辑。
- 在应用详情中展示模板依赖的配置资源和 TLS 证书状态；发布前阻止缺失资源或未就绪的证书。
- 对被模板或有效发布引用的资源增加删除保护。

## Capabilities

### Modified Capabilities

- `application-release`: 模板可选择、创建和追踪同命名空间的文件资源与 TLS 证书依赖。
- `k3s-config`: ConfigMap 与 Opaque Secret 支持受控 CRUD 和应用模板引用保护。

## Non-goals

- 不为任何具体应用增加专用配置模型或将 Secret 明文保存到部署模板。
- 不支持跨命名空间资源引用、主机路径挂载或可写配置投影。
- 不将 UDP/QUIC 转换为 HTTP Ingress。
- 不在首期实现证书私钥导入、通用 Secret 类型编辑或配置版本回滚。

## Impact

- 后端新增 ConfigMap/Secret 写入、模板引用查询和删除保护 API；发布预检扩展 TLS 证书状态检查。
- 前端配置页改为可管理资源，模板编辑器增加选择器与快捷创建弹窗，应用页新增依赖状态。
- 证书仍由现有受管域名和 cert-manager 生命周期管理，平台仅将就绪 TLS Secret 投影到工作负载。
