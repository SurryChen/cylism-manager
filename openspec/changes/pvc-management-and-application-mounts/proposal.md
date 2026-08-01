# PVC 管理与应用挂载

## Summary

为平台增加 Namespace 级 PersistentVolumeClaim（PVC）管理，并允许应用上线模板引用同一环境内的平台托管 PVC。发布渲染的 Deployment 将生成对应的 `volumes` 和 `volumeMounts`。

## Capabilities

- 新增 `persistent-storage`：按环境创建、查看和删除平台托管 PVC。
- 修改 `application-release`：上线模板可声明 PVC 挂载，发布前校验并渲染到 Deployment。
- 修改 `ui-navigation`：增加集群存储入口，并在应用模板编辑页选择可挂载 PVC。
- 修改 `cluster-node-management`：展示并安全维护节点自定义标签，作为本地存储与监控工作负载的调度约束基础。

## Goals

- 支持以默认或指定 StorageClass 创建 `ReadWriteOnce` PVC，并展示实际绑定节点与数据回收策略。
- 将 PVC 约束在一个 Environment 的 Namespace 内，避免跨环境挂载。
- 将挂载定义写入不可变 Release 快照，以便重试和回滚保持一致。
- 对写入型 RWO PVC 阻止多副本 Deployment，避免多节点挂载冲突和 SQLite 等单写入数据损坏。
- 支持在模板中选择部署节点，并以标准 hostname `nodeSelector` 对本地卷的绑定节点实施调度约束。
- 支持查看节点全部标签、维护自定义标签，并保护 Kubernetes、K3s 和节点角色等系统标签不被平台修改。

## Non-goals

- 不实现 StorageClass、PV、CSI 驱动或节点磁盘的创建和配置管理。
- 不实现 PVC 数据备份、快照、迁移、扩容或多集群复制。
- 不支持 `ReadWriteMany`、`ReadOnlyMany` 或 StatefulSet；首期专注 K3s 默认 local-path/hostpath 场景。
- 不接管未带 Cylism 管理标签的既有 PVC。

## Migration

本变更不修改已有模板或 Release 快照。未声明挂载的应用保持现有无状态发布行为；新 PVC 仅在用户显式创建和挂载后生效。
