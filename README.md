<div align="center">

# Cylism Manager

**面向运维人员的 Kubernetes 兼容基础设施控制台，将服务器、集群、交付和证书管理收敛到一处。**

[![Build](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml/badge.svg?branch=main)](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml)
[![Docs](https://img.shields.io/badge/docs-GitHub%20Pages-222?logo=github)](https://surrychen.github.io/cylism-manager/)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

[English](README.en.md) · [在线文档](https://surrychen.github.io/cylism-manager/) · [快速开始](docs/getting-started.md) · [参与贡献](CONTRIBUTING.md) · [安全说明](SECURITY.md)

</div>

<p align="center">
  <a href="docs/assets/screenshots/platform-overview.png">
    <img src="docs/assets/screenshots/platform-overview.png" alt="Cylism Manager 平台健康度概览" width="1200">
  </a>
</p>

Cylism Manager 将主机操作、Kubernetes 资源、制品交付、证书和审计记录集中到一个面向运维的 Web UI。Kubernetes 兼容 API 是基础能力；识别到 K3s 后，平台会启用针对性的优化。

> [!WARNING]
> 这是一套具备实际控制能力的平台，而不是只读仪表盘。它持有 Kubernetes 权限和受保护的 SSH 凭据；仅应部署在受信任的集群中，并遵循[安全说明](SECURITY.md)。

## 从这里开始

| 你希望... | 前往 |
| --- | --- |
| 确认平台是否适合当前环境 | [前置条件与安全边界](docs/installation/prerequisites.md) |
| 在控制面或单节点环境安装 | [部署脚本](docs/installation/install-script.md) |
| 使用既有发布流程安装 | [Helm Chart](docs/installation/install-helm.md) |
| 登录并纳管第一台服务器 | [首次使用指南](docs/getting-started.md) |
| 配置保留策略、凭据和公开入口 | [配置参考](docs/operations/configuration.md) |
| 排查安装或日常运维问题 | [运维与故障排查](docs/operations/operations.md) |
| 阅读完整文档站 | [surrychen.github.io/cylism-manager](https://surrychen.github.io/cylism-manager/) |

## 为什么选择 Cylism Manager

- **统一管理主机与集群资源。** 在同一个工作台中执行 SSH 预检和维护操作，并管理工作负载、Service、存储、证书和集群组件。
- **让交付贴近运维。** 从同一界面管理镜像仓库、自托管制品库、Registry Proxy 实例和平台版本发布。
- **让变更可追溯。** Web UI 与 REST API 会记录运维操作的审计历史；Agent 通信受到控制，而不是暴露未经认证的主机 Shell。
- **以 Kubernetes 作为通用契约。** 平台基于 Kubernetes 兼容 API 工作，不绑定某个特定发行版。

## 平台兼容性

| 范围 | 行为 |
| --- | --- |
| Kubernetes | 基础平台。Manager 通过 Kubernetes API 工作，并将 SQLite 状态存储在 PersistentVolumeClaim（PVC）中。 |
| K3s | 可选优化。仅在识别到 K3s 后，才启用 K3s Agent 节点加入和 `vpn-auth` 兼容诊断。 |
| Tailscale 与其他 VPN | 由外部系统管理。Manager 不安装、认证、注册、升级或挂载任何外部 VPN 服务。 |
| 服务器访问 | 操作员提供可达的 SSH 地址、用户和受保护凭据；地址可以来自私有网络、DNS 或其他外部维护的网络路径。 |

## 安装

请选择一种受支持的生产安装方式。两者都需要 Kubernetes 兼容集群、可为 Manager PVC 提供存储的 StorageClass，以及用于纳管服务器的 SSH 私钥。执行命令前请先阅读[前置条件](docs/installation/prerequisites.md)。

### 部署脚本

适合单节点控制面或需要交互式初始化的环境。脚本会创建或复用所需 Secret，并等待 Deployment 完成 rollout。

```bash
git clone https://github.com/SurryChen/cylism-manager.git
cd cylism-manager

bash scripts/deploy-platform.sh \
  --image ghcr.io/surrychen/cylism-manager:latest \
  --namespace cylism-system \
  --ssh-key ~/.ssh/id_ed25519 \
  --verify-image-pull
```

私有镜像、已有 Secret、备份、升级与回退检查请继续阅读[脚本安装指南](docs/installation/install-script.md)。

### Helm Chart

适合已经通过 Helm values 和受控发布流程管理 Kubernetes 应用的环境。

```bash
helm upgrade --install cylism-manager charts/cylism-manager \
  --namespace cylism-system \
  --create-namespace \
  --set image.repository=ghcr.io/surrychen/cylism-manager \
  --set image.tag=latest \
  --set secrets.existingSecret=cylism-secret \
  --set ssh.existingSecret=cylism-ssh-key
```

请在执行前创建所需 Secret。完整且适合生产环境的示例与 PVC 配置请见 [Helm 安装指南](docs/installation/install-helm.md)。

## 管理范围

| 工作区 | 示例 |
| --- | --- |
| 服务器与集群 | SSH 预检、服务器生命周期操作、节点、工作负载、Service、ConfigMap、Secret、PVC 与系统组件 |
| 交付 | 镜像仓库、自托管 OCI 制品库、Registry Proxy、应用发布和平台镜像更新 |
| 网络与安全 | 受管域名、Ingress 入口、证书、证书签发器和 DNS Challenge 集成 |
| 运维 | 审计历史、平台发布历史、健康信息、配置与诊断 |

## 范围与运行模型

- Manager 有意获得较广的 Kubernetes 权限，以协调上述资源；它不适用于仅 Namespace 级、只读或严格多租户隔离的控制面场景。
- SQLite 是单写入数据库。Manager Deployment 使用 `Recreate` 策略，升级时会短暂停止旧 Pod，再在 PVC 上启动新 Pod。
- 升级或执行破坏性操作前请备份 Manager PVC。常规升级过程中不要删除 PVC。
- K3s 支持是增量优化。标准 Kubernetes 环境无需 K3s 或 Tailscale，也可以使用通用资源与运维能力。

## 开发

前置条件：Go 1.25+、Node.js 24+ 与 npm。Docker、Helm、Python 3.10+ 和 Kubernetes 测试集群按变更范围选用。

```bash
go test ./...
go run ./cmd/platform

npm --prefix web ci
npm --prefix web run dev
```

构建项目：

```bash
make build
```

OpenSpec 变更流程、审查要求和完整验证集请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 项目结构

```text
cmd/        Go 程序入口
internal/   API、服务、数据存储和 Kubernetes 集成
web/        Vue 3 管理界面
k8s/        静态 Kubernetes 清单
charts/     Helm Chart
scripts/    安装与部署辅助脚本
docs/       公开使用文档与历史设计材料
openspec/   变更规范与归档决策
```

## 许可证

Cylism Manager 采用 [Apache License 2.0](LICENSE)。
