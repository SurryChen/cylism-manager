## Why

节点镜像源页面需要同时支持规则维护、选节点下发和节点实际配置排障。此前将规则改成多列卡片后，页面与现有工作台的列表模式不一致，横向信息密度过高；节点配置检查也不应占据主页面。用户需要从顶部入口选择任一节点，查看其脱敏配置和差异，并在明确确认后重启该节点的 K3s 服务。

## What Changes

- 将节点镜像源主界面改为与既有工作台一致的单行规则列表；每条规则只展示摘要和操作，不在列表中展开节点应用日志。
- 在顶部提供“查看节点配置”入口。弹窗中选择任一集群节点后，通过受控 SSH 读取其 `registries.yaml`，解析、脱敏并与当前全部启用规则比较。
- 在同一节点配置弹窗中提供显式确认后的 K3s 服务重启。重启仅检测并重启 `k3s.service` 或 `k3s-agent.service`，不会重启宿主机，也不会写入 Registry 配置。
- 将节点应用结果收纳为每条规则的“查看记录”弹窗；不在主列表直接展示长日志。
- 将“管理 Registry Proxy”跳转保留为链接按钮，但移除默认文本下划线。

## Capabilities

### New Capabilities

- `node-registry-mirror-observation`: 安全采集、脱敏比较并展示集群节点实际 K3s Registry 配置，以及围绕该状态组织镜像源工作台。

### Modified Capabilities

- None.

## Impact

- 修改节点镜像源的 REST 路由、Handler、Registry SSH 服务、DTO、路由金丝雀测试与页面 API 客户端。
- 修改 `NodeRegistryMirrors.vue` 和其测试；保留数据库模型、现有镜像源 API、下发协议与 Proxy 生命周期 API。
- 只复用现有 SSH 执行通道、YAML 库、确认弹窗和设计 token；不引入第三方依赖、不持久化实际节点配置、不重启宿主机。
