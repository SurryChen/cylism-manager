# API Architecture Baseline

建立日期：2026-08-29

基线提交：`2df2196163ec78efb12b7f8c007c77f0d32af416`

本文件记录 API 分层迁移前的结构和验证结果。后续每个功能模块迁移完成后，应使用相同命令重新验证，并确认行为没有回归。

## 当前结构

`internal/api` 当前仍是单一 Go package：

- 47 个生产 Go 文件
- 32 个测试文件
- 约 26,897 行 Go 代码（包含测试）
- 308 个路由注册点，集中在 `internal/api/router.go`

当前已存在可复用的相邻层：

- `internal/application`：应用发布领域服务和资源渲染
- `internal/k8s`：Kubernetes 客户端及资源操作适配
- `internal/model`：持久化模型和响应模型
- `internal/store`：SQLite/GORM 持久化访问
- `internal/agent`、`internal/auth`、`internal/crypto`：独立的基础能力

## 高优先级耦合点

以下文件同时承担 HTTP Handler、业务规则、持久化访问或 Kubernetes 编排，优先作为后续拆分候选：

| 文件 | 行数 | 首选迁移领域 |
| --- | ---: | --- |
| `internal/api/application_handler.go` | 2350 | application |
| `internal/api/delivery/managed_registry_handler.go` | 1186 | delivery / registry |
| `internal/api/agent_handler.go` | 1137 | agent |
| `internal/api/k8s_handler.go` | 1083 | infrastructure |
| `internal/api/alerting_handler.go` | 977 | system / monitoring |
| `internal/api/server_handler.go` | 812 | infrastructure |
| `internal/api/system_component_handler.go` | 754 | system |
| `internal/api/integration_application_handler.go` | 729 | application |
| `internal/api/registry_proxy_handler.go` | 695 | delivery / registry |
| `internal/api/platform_handler.go` | 651 | delivery / platform |
| `internal/api/logging_handler.go` | 618 | system / monitoring |
| `internal/api/runtime_handler.go` | 610 | agent / runtime |
| `internal/api/pvc_migration_handler.go` | 554 | infrastructure / storage |
| `internal/api/domain_handler.go` | 550 | infrastructure / network |
| `internal/api/node_handler.go` | 523 | infrastructure / cluster |

这些数字用于确定拆分优先级，不代表必须按文件整体移动。迁移时应先提取 Service、Repository 或 K8s Adapter，再移动 Handler。

## 路由领域概览

`router.go` 当前注册的主要领域包括：

- 认证：`/api/auth`
- Agent 和运行时：`/api/agent/v1`、`/api/runtimes`
- 集群基础设施：`/api/servers`、`/api/nodes`、`/api/k8s`
- 制品和镜像：`/api/image-registries`、`/api/node-registry-mirrors`、`/api/managed-oci-registries`、`/api/registry-proxy`、`/api/chart-repositories`
- 平台和应用交付：`/api/platform`、`/api/projects`、`/api/applications`
- 网络和证书：`/api/routes`、`/api/certs`、`/api/tailscale`、`/api/domains`
- 系统运维：`/api/system-components`、`/api/monitoring`、`/api/monitoring/alerts`、`/api/monitoring/logs`、`/api/audit-logs`

迁移时保持这些外部路径不变；只调整内部 Handler 的组织和调用关系。

## 验证结果

执行环境：

```text
Go go1.26.4 darwin/arm64
Node v16.20.2
npm 8.19.4
```

### Go 测试

```bash
go test ./...
```

结果：通过。所有 Go package 测试成功，包括 `internal/api`、`internal/application`、`internal/k8s`、`internal/store` 等。

### Go 构建

```bash
go build ./...
```

结果：通过。首次沙箱执行因 Go 模块缓存写权限失败，随后在无沙箱环境重跑通过。

### 前端构建

```bash
cd web
npm run build
```

结果：通过。

Vite 输出当前存在一个非阻断提示：生产 JavaScript bundle 压缩后约 1.23 MB，超过默认 500 kB 建议阈值。该提示属于已有前端构建优化事项，不作为本次 API 分层迁移阻塞项。

## 阶段 0 结论

1. 当前代码基线可编译、可测试、可构建。
2. API 路由契约可作为迁移期间的稳定边界。
3. 自托管制品库已按 `api -> service -> repository/k8s` 方向完成迁移，保留原 REST 路由。
4. 后续模块完成后必须重复本文件中的三项验证，再进入下一个模块。
