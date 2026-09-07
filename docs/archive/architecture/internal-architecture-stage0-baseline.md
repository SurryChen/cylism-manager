# Internal Architecture Stage 0 Baseline

日期：2026-08-30
基线提交：`e4a5c04`

## 范围

阶段 0 只处理基线、明确的低风险问题和共享 HTTP 辅助能力，不改变 REST 路径、鉴权、错误码、数据库结构或 Kubernetes 资源行为。

## 目录与复杂度快照

当前 `internal/api` 共 32 个根目录 Go 文件，生产代码和测试合计 25,470 行。主要需要后续拆分的生产文件如下：

| 文件 | 行数 | 后续方向 |
|---|---:|---|
| `internal/api/application_handler.go` | 2,073 | 按项目、应用、发布、模板和 Endpoint 拆分 |
| `internal/api/agent/agent_handler.go` | 1,140 | 继续按 Agent 运行时和操作边界拆分 |
| `internal/api/infrastructure/k8s_handler.go` | 1,104 | 按 Namespace、Workload、Service、ConfigMap、Secret 等资源族拆分 |
| `internal/api/system/alerting_handler.go` | 978 | 将告警查询、通知和自动化移入 Service |
| `internal/api/system/system_component_handler.go` | 754 | 将组件状态和修复逻辑移入 Service |
| `internal/api/system/logging_handler.go` | 618 | 将日志查询和结果归一化移入 Observability Service |
| `internal/api/system/monitoring_handler.go` | 508 | 将监控查询和分析移入 Observability Service |
| `internal/api/router.go` | 527 | 后续按领域拆分路由注册函数 |

当前 Router 注册了 29 个领域分组，覆盖 auth、runtime、system、cluster、registry、platform、application、infrastructure、monitoring、admin 和 tailscale 等路径。路由迁移时必须以 `internal/api/router.go` 的现有路径和 HTTP 方法为契约。

## 共享辅助函数盘点

阶段 0 前，以下逻辑在多个 API 包中重复实现：

- `getUserID`：根包、agent、system、delivery 和 infrastructure。
- `parseID`：根包、delivery，以及 infrastructure 中带 Gin 参数读取的变体。
- `k8sUnavailable`：根包、system 和 infrastructure，且文案曾分别为 `K8s`、`k8sClient`、`h.k8s`。

本阶段新增 `internal/api/shared`：

- `UserID(*gin.Context) uint`
- `ParseID(string) (uint, error)`
- `K8sUnavailable(*gin.Context)`

所有领域调用点已直接使用 shared 函数，原有 `auth_helpers.go`、`k8s_errors.go` 和同名包内 helper 已删除。基础设施资源 ID 使用 `ParsePositiveID` 保留原先拒绝零值的校验。`K8sUnavailable` 的响应统一为 HTTP 200、错误码 `50101`、文案 `K8s 集群未连接`。

## 基线验证

以下命令在阶段 0 改动前执行通过：

```text
go test ./...                         PASS
go build ./...                        PASS（需允许 Go 写入模块缓存）
cd web && nvm use 24 && npm run build PASS
```

前端构建仍有 Vite 的 chunk size warning，但不影响构建结果。阶段 0 改动后再次执行同一组验证，并额外执行：

```text
go test ./internal/api/shared ./internal/api/... PASS
git diff --check                                  PASS
```

## 阶段 0 验收结论

- [x] 完成目录、复杂度和路由基线记录。
- [x] 完成身份、ID 解析和 Kubernetes 不可用响应的共享归属。
- [x] 修正 system 包不一致的 Kubernetes 未连接文案。
- [x] 为共享 helper 增加缺失、异常身份值、ID 解析和错误响应测试。
- [x] 未改变现有 API 路径、请求/响应字段和业务资源行为。

阶段 1 可从 Application API 的领域拆分开始。Store、Kubernetes Adapter 和 Observability 的结构性改造不属于本阶段，避免基线阶段引入大范围行为风险。
