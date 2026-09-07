## Why

应用模板虽然在后端发布定义中已具备 Kubernetes `command` 与 `args` 字段，但编辑器没有配置入口。需要显式启动参数的服务只能依赖镜像默认命令，无法可靠部署。

发布详情目前只反映一次编排过程。Pod 随后因退出、重启或调度问题失去可用性时，用户必须离开平台手工使用 kubectl 排查，且历史 Release 无法准确识别其创建的 Pod。

## What Changes

- 在应用上线模板中增加启动命令与启动参数编辑能力，按 Kubernetes 字符串数组语义保存和下发。
- 将发布序号标签写入 Deployment Pod 模板，使每次发布的 Pod 可与 Release 精确关联。
- 扩展发布详情 API，实时返回关联 Pod 的状态、容器状态、重启信息与脱敏诊断。
- 在发布详情页展示 Pod 运行态，并在 Release 已完成后继续刷新运行态。

## Capabilities

### Modified Capabilities

- `application-release`: 模板运行命令配置、Release 与 Pod 关联、运行态诊断和详情展示。

## Non-goals

- 不执行 Shell，不实现命令行字符串、引号或环境变量展开。
- 不将短生命周期 Pod 状态复制或持久化至 SQLite。
- 不改变 Release 的历史结果；已成功的 Release 运行态异常以实时健康信息展示。
- 不实现日志流、Pod 终端或自动修复重启。

## Impact

- 后端扩展发布资源标签、发布详情查询与 Kubernetes Pod/Event 诊断。
- 前端模板编辑器增加两个字段，发布详情增加运行态面板与轮询策略。
- 需要现有 ServiceAccount 保有 Pods 与 Events 的 list/get 权限。
