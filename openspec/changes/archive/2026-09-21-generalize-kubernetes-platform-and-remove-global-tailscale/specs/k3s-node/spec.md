## MODIFIED Requirements

### Requirement: 节点添加
系统 SHALL 仅允许从“已激活且未加入当前集群”的服务器中选择目标，并且仅在已识别为 K3s 的集群中通过 SSH 安装 K3s worker；系统 SHALL 使用操作员配置且从目标服务器可达的 control-plane 地址与 K3s join token，且 SHALL 不支持通过 UI 新增第二个 control-plane。

#### Scenario: 从已激活服务器加入 worker
- **WHEN** 用户在已识别为 K3s 的集群节点页面选择一台状态为 `ready` 的服务器并触发“加入集群”
- **THEN** 系统使用该服务器的 SSH 凭据执行固定前置检查，并使用配置的 control-plane 地址作为 `--server=https://...:6443` 安装 K3s agent
- **AND THEN** 系统 SHALL NOT 安装、注册、调用或读取 Tailscale 及其 Auth Key

#### Scenario: 非 K3s 集群不能加入 worker
- **WHEN** 当前集群被识别为 `kubernetes` 或 `unknown`，且用户或客户端尝试启动节点加入
- **THEN** 系统拒绝操作并返回明确的 K3s capability 不可用结果
- **AND THEN** 前端 SHALL 不展示可执行的加入节点入口

#### Scenario: 非激活服务器不能加入
- **WHEN** 用户尝试对 `discovered`、`credential_pending` 或 `connectivity_failed` 状态的服务器执行加入集群
- **THEN** 系统拒绝操作，并提示必须先完成激活检测

### Requirement: 节点列表
系统 SHALL 展示当前 Kubernetes 集群中的节点列表，并以 Kubernetes API 为唯一事实来源；系统 SHALL 在页面上显示与服务器台账的映射关系，但不得依赖 Tailscale IP。

#### Scenario: 查看节点列表
- **WHEN** 用户访问集群节点页面
- **THEN** 系统从 Kubernetes API 返回所有 Node，包含名称、角色、Ready 状态、Kubernetes 版本、InternalIP、CPU、内存容量
- **AND THEN** 若 Node 名称与服务器记录的 `k8s_node_name` 匹配，则额外显示对应服务器名称

#### Scenario: 节点映射失败
- **WHEN** 某个 Node 名称无法与任一服务器记录匹配
- **THEN** 页面将该节点标记为“未映射”或 `drifted`
