# Cylism Manager

Cylism Manager 是一个面向 Tailscale + 单节点 K3s 场景的基础设施控制台。
它负责服务器纳管、K3s 资源管理、平台自更新、审计和一些常见运维能力。

## 主要能力

- 通过 Tailscale 发现和管理主机
- 通过 SSH 完成服务器预检、加入集群和常见维护
- 管理 K3s / Kubernetes 资源
- 管理平台自身的发布与回滚
- 提供 Web UI、REST API 和 Agent 通信能力

## 仓库结构

- `cmd/`：Go 程序入口
- `internal/`：后端核心实现
- `web/`：Vue 3 前端
- `k8s/`：当前部署清单
- `charts/`：Helm Chart
- `scripts/`：部署和初始化脚本
- `docs/`：部署、设计和迁移文档
- `openspec/`：OpenSpec 变更与规范
- `config/`：示例配置
- `PRODUCT.md`：产品定义与定位说明

## 快速开始

前置条件：

- Go 1.22+
- Node.js 22+
- Docker
- 可选：K3s / kubectl

后端开发：

```bash
go test ./...
go run ./cmd/platform
```

前端开发：

```bash
cd web
npm install
npm run dev
```

构建：

```bash
make build
```

## 部署

推荐先看：

- [docs/k3s-tailscale-deployment.md](docs/k3s-tailscale-deployment.md)

常用脚本：

- `scripts/deploy-platform.sh`：当前推荐的安装 / 升级脚本
- `scripts/init-k3s.sh`：初始化 K3s 和平台资源
- `scripts/install-tailscale.sh`：安装 Tailscale
- `scripts/deploy.sh`、`scripts/dev-deploy.sh`：历史维护脚本，保留作参考

GitHub Release 会生成：

- `cylism-manager-deploy-*.tar.gz`：部署包
- `cylism-manager-*.tgz`：Helm Chart

## 配置

运行时配置主要通过环境变量和 Kubernetes Secret / ConfigMap 注入。
示例配置可参考：

- `config/config.example.yaml`

敏感信息不要提交到仓库。

## 文档入口

- [PRODUCT.md](PRODUCT.md)
- [docs/k3s-tailscale-deployment.md](docs/k3s-tailscale-deployment.md)
- [docs/design/cylism-manager-k3s-infra-console.md](docs/design/cylism-manager-k3s-infra-console.md)

