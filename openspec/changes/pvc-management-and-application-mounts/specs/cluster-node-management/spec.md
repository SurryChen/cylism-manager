## ADDED Requirements

### Requirement: 节点自定义标签管理
系统 SHALL 展示 Kubernetes Node 标签，并允许用户维护不属于平台保护前缀的自定义标签。

#### Scenario: 查看节点标签
- **WHEN** 用户在集群页查看某个 Kubernetes 节点
- **THEN** 系统展示该节点的所有标签和键值
- **AND** 清晰区分系统标签与可维护的自定义标签

#### Scenario: 维护自定义标签
- **WHEN** 用户为节点设置、更新或移除符合 Kubernetes 标签格式的自定义标签
- **THEN** 系统更新对应 Node 的 labels
- **AND** 集群页刷新后展示实际保存的标签

#### Scenario: 保护系统标签
- **WHEN** 用户尝试修改或删除 `kubernetes.io/*`、`node.kubernetes.io/*`、`k3s.io/*`、`node-role.kubernetes.io/*` 或 `beta.kubernetes.io/*` 标签
- **THEN** 系统拒绝请求并说明该标签由 Kubernetes 或 K3s 管理
- **AND** 不修改节点标签

### Requirement: 基于标签的本地工作负载调度
系统 SHALL 对选择部署节点的应用 PVC 工作负载和 VictoriaMetrics hostPath 工作负载使用标准 Node selector，而非直接绑定 PodSpec nodeName。

#### Scenario: 应用本地 PVC 使用 hostname selector
- **WHEN** 应用模板选择 Kubernetes 节点 `node-a` 并引用本地 PVC
- **THEN** 系统生成 `nodeSelector.kubernetes.io/hostname=node-a`
- **AND** 系统不设置 PodSpec nodeName

#### Scenario: VictoriaMetrics 使用 hostname selector
- **WHEN** 用户选择 Kubernetes 节点 `node-a` 安装 VictoriaMetrics
- **THEN** 系统生成 `nodeSelector.kubernetes.io/hostname=node-a` 的 VictoriaMetrics Deployment
- **AND** 系统不设置 PodSpec nodeName
