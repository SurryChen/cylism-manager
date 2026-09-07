# Design: Application Multi-Domain Endpoints

## Model

`ApplicationEndpoint` 保持一行代表一个 `domain_id + path` 入口。现有表没有应用 ID 唯一约束，因此无需 SQLite schema migration；已有记录自然成为列表中的第一项。

每个入口仍只允许引用当前应用环境中已启用的受管域名。HTTPS 入口必须引用该域名已就绪的 Certificate 生成的 TLS Secret。一个应用可以绑定多个不同 Host，也可以在同一 Host 下绑定多个不同路径；同一个 `domain_id + path` 在整个命名空间中只能归属一个应用。

## API

单例接口替换为列表接口：

```text
GET    /api/applications/:id/endpoints
POST   /api/applications/:id/endpoints
PUT    /api/applications/:id/endpoints/:endpointID
DELETE /api/applications/:id/endpoints/:endpointID
```

创建和编辑请求继续使用 `domain_id`、`path` 与 `tls_enabled`。服务端根据受管域名填充 Host、TLS Secret、Service Port 和 Issuer 元数据，浏览器不能提交这些派生字段。

编辑与删除必须确认 `endpointID` 属于 URL 中的应用。冲突检查按入口 ID 排除当前记录，而不能再按应用 ID 排除全部入口。

## Ingress Synchronization

每个应用始终最多拥有一个以应用名命名、带平台管理标签的 Ingress。同步步骤读取该应用当前全部入口，并构建：

- 每个入口一条 `IngressRule`，Host 为受管域名，Path 指向应用 Service 的当前端口。
- 每个 TLS Secret 一条 `IngressTLS`；使用相同证书的 Host 合并到同一条 TLS 配置。

新增、编辑或删除入口后，平台先完成业务校验与数据变更，再从数据库重新读取入口列表同步 Ingress。删除最后一个入口时，删除受管 Ingress。Kubernetes 同步失败将返回错误，前端重新读取列表和 Ingress 实际状态后可重试；数据库不保存 Kubernetes 资源快照。

## Release Compatibility

域名绑定是应用级配置，不是版本模板字段。发布、重试和回滚仍从模板构建工作负载和 Service，但不再用 `ReleaseSpec.Endpoint` 单独渲染或覆盖 Ingress；在资源应用后统一按应用入口列表同步。

旧 Release 中保留的单入口快照仅作为历史记录，不用于重建当前 Ingress。这样新增多个域名后再次发布不会丢失已有规则，解绑域名也不会被旧版本重新引入。

## Console

应用详情的“对外访问”区域显示入口列表：域名、路径、HTTPS 状态与 TLS Secret。用户可新增绑定、编辑路径/TLS 或仅解绑选中的一项。选择器只显示当前环境中证书已就绪的受管域名。

应用列表继续将第一个入口作为新标签页链接，并在存在额外入口时展示数量。点击该链接不触发行点击进入应用详情。

## Security And Safety

- 域名与 TLS Secret 始终从受管域名资产派生，不接受客户端自由输入。
- 域名路径冲突由服务端在写入前检查，防止两应用争夺同一路由。
- 所有 Ingress 更新先验证资源由平台管理，避免覆盖同名外部 Ingress。
- 删除入口只影响对应规则；最后一个入口删除才移除 Ingress。
