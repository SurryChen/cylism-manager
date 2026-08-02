## Context

`application.ReleaseSpec` 已包含 `Command []string` 和 `Args []string`，`RenderResources` 也已写入 `corev1.Container`。模板 JSON 因此无需数据库迁移；问题在于 UI 未创建或编辑这些字段。

现有 Release 只有 Deployment 的 `cylism.io/release` annotation，而 Pod template 没有该 label。应用名标签可以找到当前应用的 Pod，却不能在多次发布后区分它们属于哪次 Release。

## Decisions

### 1. 逐行表达 Kubernetes 字符串数组

模板编辑器使用两个 textarea。每个非空行成为数组中的一个元素：命令输入对应 `command`，参数输入对应 `args`。保存时丢弃纯空行并保留元素中的普通空格；展示和编辑时按行还原。

不解析完整 Shell 命令。Kubernetes 的 `command`/`args` 本来就是 argv 数组，Shell 解析会引入无法预期的转义语义及潜在的注入误解。

### 2. 以不可变 Release 序号标注 Pod

在每次渲染资源时，将 `cylism.io/release=<sequence>` 加入 Deployment 的 Pod template labels。Deployment selector 继续仅使用应用稳定标签，滚动更新不受影响。Release sequence 已在应用内唯一，因此可作为该应用 Pod 的关联键。

早于此版本的 Release 没有 Pod 标签。查询时应返回明确的 `legacy_untracked` 状态，而不是用应用名标签猜测归属。

### 3. Pod 运行态从 Kubernetes 实时读取

Release 详情 API 根据应用 Namespace、应用名和发布标签 List Pod，并映射为仅包含所需字段的响应对象。每个容器返回当前状态、上次终止状态、就绪状态及重启次数；不返回环境变量、挂载内容、完整事件对象或敏感配置。

诊断优先级：Pod Failed、Waiting、当前/上次 Terminated、未调度条件、关联 Warning Event。`CrashLoopBackOff` 且最近一次 `Completed` 将明确描述为“进程正常退出但被持续重启”。错误内容复用发布诊断脱敏逻辑。

### 4. 发布成功与当前运行态分离

Release `succeeded` 只说明该次编排完成时工作负载曾达到就绪条件。`runtime` 是当前事实，不回写历史 Release 状态。前端在 Release 终态后也每 5 秒刷新详情，使运行态异常可见；离开页面时停止轮询。

## Alternatives Considered

- 将当前 Pod 状态写入 Release 表：会立即过期，且无法代表重建后的 Pod。
- 用 ReplicaSet revision 关联 Release：Deployment controller 管理的 revision 非平台 Release ID，回滚和手工变更会使映射不可靠。
- 用应用标签显示所有 Pod：无法区分新旧发布，可能把其他 Release 的异常错误归因给当前 Release。
- 终态 Release 不再轮询：不能发现本次问题中的短暂就绪后重启。

## Risks and Mitigations

- Pod 已被替换：返回空列表和“当前无关联 Pod”，不显示过期快照。
- 事件含敏感内容：应用发布相关凭据在渲染响应前脱敏并限制详情长度。
- 旧 Release 无标签：返回明确兼容状态，下一次发布后才可精确关联。
- Kubernetes 查询失败：保留 Release 基础信息并将运行态错误独立返回，避免详情整体失败。
