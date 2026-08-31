# Internal Architecture Stage 1 Baseline

日期：2026-08-30

## 范围

阶段一只拆分 Application API 的物理文件边界，保持 `package api`、`NewApplicationHandler`、Gin 路由、REST 请求/响应、数据库结构和 Kubernetes 行为不变。

## 文件结果

| 文件 | 行数 | 职责 |
|---|---:|---|
| `internal/api/application_handler.go` | 217 | Handler 依赖、构造和应用生命周期 |
| `internal/api/project_handler.go` | 121 | 项目 CRUD |
| `internal/api/environment_handler.go` | 329 | 环境 CRUD 和命名空间绑定 |
| `internal/api/runtime_view.go` | 302 | 工作台、发现和运行态响应映射 |
| `internal/api/release_handler.go` | 242 | 发布、重启、重试、回滚和发布详情 |
| `internal/api/template_handler.go` | 376 | Deployment Template CRUD 和默认模板 |
| `internal/api/endpoint_handler.go` | 334 | Application Endpoint CRUD 和入口同步 |

原 `application_handler.go` 为约 2,073 行，拆分后最大文件为 538 行，所有目标 Handler 文件均低于 800 行。

## 兼容性检查

- Router 仍通过 `NewApplicationHandler(...).WithDelegationSecret(...)` 创建同一个 Handler。
- `integration_application_handler.go` 使用的模板转换方法仍在同一 package 内可复用。
- 发布流程仍由 `application.ReleaseWorkflow` 执行，未在 HTTP Handler 中复制状态机。
- 工作台、发现和集成读取路径统一通过 `internal/service/application.QueryService` 获取项目、环境、应用和发布数据；该服务提供环境归属解析，并批量完成 Kubernetes Service、工作负载和 Pod 的运行态摘要组装，只依赖最小只读 Store 接口，便于使用 Stub 测试。
- 应用、项目、环境、运行态、发布、模板、Endpoint 和集成入口测试已拆分至对应的 `*_handler_test.go`；`application_handler_test.go` 仅保留应用生命周期测试和共享 Router fixture。
- Application 领域 Handler 不再直接调用 Store 的项目、环境、应用、应用列表或发布列表读取方法；模板、Endpoint 和受管文件仍通过其专属 Store 查询。

## 验证

```text
go test ./...       PASS
go build ./...      PASS（需允许 Go 写入模块缓存）
git diff --check    PASS
```

前端未修改；阶段完成验证仍执行既有前端构建命令。
