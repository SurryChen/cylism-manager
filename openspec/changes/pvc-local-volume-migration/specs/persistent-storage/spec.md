## ADDED Requirements

### Requirement: 结构化 PVC 容量输入
系统 SHALL 在创建平台托管 PVC 时将容量数值与 Kubernetes 二进制单位分开输入，并向 Kubernetes 提交合法 Quantity。

#### Scenario: 创建 Gi 容量的 PVC
- **WHEN** 用户填写容量数值 `5` 并选择单位 `Gi`
- **THEN** 前端向 PVC 创建 API 提交存储容量 `5Gi`
- **AND** PVC 列表继续显示 Kubernetes 返回的容量 Quantity

#### Scenario: 拒绝无效容量数值
- **WHEN** 用户未填写正整数容量或选择不受支持的单位
- **THEN** 前端阻止提交并展示字段校验状态
- **AND** 不调用 PVC 创建 API

### Requirement: 本地 PVC 迁移入口和状态展示
系统 SHALL 在受管本地 PVC 列表中展示迁移入口、进行中状态、目标节点和成功后的源卷清理入口。

#### Scenario: 显示可迁移本地卷
- **WHEN** 受管 PVC 已 Bound 且具有本地 PV 节点亲和性
- **THEN** 列表展示当前绑定服务器和迁移操作
- **AND** 迁移对话框仅列出不同于源节点的可用目标节点

#### Scenario: 显示迁移中的卷
- **WHEN** PVC 存在未终态迁移任务
- **THEN** 列表展示当前迁移阶段和步骤详情入口
- **AND** 禁用该 PVC 的删除与重复迁移操作
