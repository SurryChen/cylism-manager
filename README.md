# Cylism Manager

Cylism Manager 是面向 Tailscale + K3s 的基础设施控制台。它把服务器纳管、SSH 主机操作、Kubernetes 资源、制品库、证书与审计记录收敛到一个界面。

> Cylism Manager 会访问 Kubernetes API、控制面 Tailscale socket，并使用受保护的 SSH 凭据操作主机。请只在受信任的控制面节点和 Kubernetes 集群中部署。

## 主要能力

- 通过 Tailscale 发现和管理主机。
- 通过 SSH 完成服务器预检、纳管与常见维护操作。
- 管理 K3s / Kubernetes 资源、存储、证书和集群组件。
- 管理镜像仓库、制品库与 Registry Proxy。
- 提供 Web UI、REST API、审计记录和受控 Agent 通信能力。

## 文档与安装

| 场景 | 文档 |
| --- | --- |
| 在线文档站 | `https://<owner>.github.io/cylism-manager/` |
| 安装前置条件 | [docs/installation/prerequisites.md](docs/installation/prerequisites.md) |
| 脚本安装 | [docs/installation/install-script.md](docs/installation/install-script.md) |
| Helm 安装 | [docs/installation/install-helm.md](docs/installation/install-helm.md) |
| 首次使用与服务器纳管 | [docs/getting-started.md](docs/getting-started.md) |
| 安全边界与漏洞报告 | [docs/security.md](docs/security.md) / [SECURITY.md](SECURITY.md) |

当前公开支持的生产安装方式为 K3s 部署脚本和 Helm Chart；不提供 Docker Compose 生产安装路径。

## 开发

前置条件：Go 1.25+、Node.js 24+；可选 Docker、Helm 和 K3s 测试集群。

```bash
go test ./...
go run ./cmd/platform

npm --prefix web ci
npm --prefix web run dev
```

构建全部二进制：

```bash
make build
```

贡献流程和完整验证命令见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 仓库结构

- `cmd/`：Go 程序入口
- `internal/`：后端核心实现
- `web/`：Vue 3 前端
- `k8s/`：Kubernetes 清单
- `charts/`：Helm Chart
- `scripts/`：部署与初始化脚本
- `docs/`：公开使用文档与历史设计记录
- `openspec/`：OpenSpec 规范与变更记录
- `config/`：示例配置

## 许可证

本项目采用 [Apache License 2.0](LICENSE)。
