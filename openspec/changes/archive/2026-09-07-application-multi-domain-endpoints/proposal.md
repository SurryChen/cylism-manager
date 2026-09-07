# Application Multi-Domain Endpoints

## Why

一个应用当前只能维护一条对外入口。重新绑定域名会删除旧入口，并在后续发布、重试或回滚时再次以单一 Host 重建 Ingress。实际服务经常需要同时提供主域名、别名域名或不同 Host 的兼容入口，现有行为既无法满足需求，也存在后续发布覆盖人工配置的风险。

## What Changes

- 将应用入口从单例记录改为应用级列表，一个应用可绑定多个受管域名和路径。
- 以一个受平台管理的 Ingress 合并该应用的全部 Host 规则和 TLS Secret 引用。
- 提供按入口 ID 的查询、创建、编辑和解绑 API；已有单入口记录自动成为列表中的一项。
- 所有应用发布、重试与回滚均从当前入口列表同步 Ingress，避免重新发布覆盖其他域名。
- 应用详情页展示入口列表并提供绑定、编辑、解绑操作；应用列表保留主入口快捷链接和额外入口计数。

## Non-Goals

- 不支持自由输入未经平台管理的域名、TLS Secret 或 Ingress 注解。
- 不管理业务 DNS A/AAAA/CNAME 解析。
- 不实现跨命名空间共享同一 Host。
- 不改变应用 Service、工作负载或受管域名证书的生命周期。

## Impact

- 修改 `internal/store` 的入口查询和按 ID 更新/删除能力。
- 修改应用入口 API、发布端点继承与 Kubernetes Ingress 渲染/同步。
- 修改应用详情页、工作台应用链接和对应 Go/Vue 测试。
- 复用现有受管域名、Certificate 和 TLS Secret 校验，不需要新增 Kubernetes 权限或数据迁移。
