# k3s-node Specification

## Purpose
TBD - created by archiving change k3s-native-arch. Update Purpose after archive.
## Requirements
### Requirement: 节点添加
系统 SHALL 仅允许从“已激活且未加入当前集群”的服务器中选择目标，通过 SSH 安装 K3s worker，并将其加入当前 control-plane；系统 SHALL 不支持通过 UI 新增第二个 control-plane。

#### Scenario: 从已激活服务器加入 worker
- **WHEN** 用户在集群节点页面选择一台状态为 `ready` 的服务器并触发“加入集群”
- **THEN** 系统使用该服务器的 Tailscale IPv4 作为 `--node-ip` 安装 K3s agent
- **AND** 系统使用 control-plane 的 Tailscale IPv4 作为 `--server=https://...:6443`

#### Scenario: 非激活服务器不能加入
- **WHEN** 用户尝试对 `discovered`、`credential_pending` 或 `connectivity_failed` 状态的服务器执行加入集群
- **THEN** 系统拒绝操作，并提示必须先完成激活检测

### Requirement: 节点列表
系统 SHALL 展示当前 K3s 集群中的节点列表，并以 Kubernetes API 为唯一事实来源；系统 SHALL 在页面上显示与服务器台账的映射关系。

#### Scenario: 查看节点列表
- **WHEN** 用户访问集群节点页面
- **THEN** 系统从 K8s API 返回所有 Node，包含名称、角色、Ready 状态、K8s 版本、InternalIP、CPU、内存容量
- **AND** 若能根据 `tailscale_ipv4 == InternalIP` 映射到服务器，则额外显示对应服务器名称

#### Scenario: 节点映射失败
- **WHEN** 某个 Node 的 InternalIP 无法与任一服务器记录匹配
- **THEN** 页面将该节点标记为“未映射”或 `drifted`

### Requirement: 节点驱逐
系统 SHALL 支持安全驱逐节点上的工作负载。

#### Scenario: 驱逐节点
- **WHEN** 用户对某工作节点触发驱逐
- **THEN** 系统执行 kubectl drain，将 Pod 迁移到其他节点

### Requirement: 节点删除
系统 SHALL 支持从集群中移除节点。

#### Scenario: 删除节点
- **WHEN** 用户确认删除已驱逐的节点
- **THEN** 系统调用 K8s API 删除 Node 资源，节点从列表移除

### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: 服务器与节点分离展示
前端 SHALL 将“服务器台账”和“集群节点”拆分为两个独立页面，而不是在同一页面通过页签混合展示。

#### Scenario: 打开服务器页面
- **WHEN** 用户访问 `/servers`
- **THEN** 页面展示服务器台账、纳管状态和 SSH 相关操作
- **AND** 不展示 K8s Node 主表格

#### Scenario: 打开集群节点页面
- **WHEN** 用户访问 `/cluster`
- **THEN** 页面展示 K8s Node 列表、节点操作和节点状态详情

