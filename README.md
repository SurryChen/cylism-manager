<div align="center">

# Cylism Manager

**把自托管里那些零碎、容易忘、却总得处理的事，放到一个地方。**

[![Build](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml/badge.svg?branch=main)](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml)
[![Docs](https://img.shields.io/badge/docs-GitHub%20Pages-222?logo=github)](https://surrychen.github.io/cylism-manager/)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

[English](README.en.md) · [在线文档](https://surrychen.github.io/cylism-manager/) · [快速开始](docs/getting-started.md) · [参与贡献](CONTRIBUTING.md) · [安全说明](SECURITY.md)

</div>

一套小集群运行久了，机器在 SSH 里，资源在 `kubectl` 里，发布、域名、证书和日志又散在不同入口。

Cylism Manager 是我为个人项目、小型服务和 homelab 做的一个自托管工作台。它把可通过 SSH 访问的 Linux 服务器、Kubernetes 集群和正在运行的服务放到同一个界面里，让日常维护少一些来回切换。

<p align="center">
  <a href="docs/assets/screenshots/platform-overview.png">
    <img src="docs/assets/screenshots/platform-overview.png" alt="Cylism Manager 平台健康度概览" width="1200">
  </a>
</p>

## 它现在能帮上什么

**先知道发生了什么。**

看服务器、节点、服务和近期操作，先定位真正需要处理的地方。

**再动手处理。**

通过 SSH 做主机预检和维护，也可以在界面里处理常见的 Kubernetes 资源。

**最后把服务发出去。**

管理镜像和发布记录，部署应用，配置域名与证书，让服务真正可访问。

## 它不是

它不是多租户云平台，也不是用来替代所有 Kubernetes 工具的抽象层。它更适合拥有几台服务器、一个 K3s 或 Kubernetes 集群，并且希望把自己的服务维持得更轻松的人。

Kubernetes 是基础能力，K3s 只是额外优化。Tailscale 和其他 VPN 由外部系统维护，Manager 不负责安装、认证、注册或升级它们。

> [!WARNING]
> Manager 不是只读仪表盘。它需要 Kubernetes 管理权限，也会持有受保护的 SSH 凭据。请只在你信任的集群中部署，并阅读[安全说明](SECURITY.md)。

## 开始使用

先阅读[安装前置条件](docs/installation/prerequisites.md)，确认集群有可用的 PVC 存储，并准备好用于纳管服务器的 SSH 私钥。

- 想快速跑起来：[部署脚本](docs/installation/install-script.md)
- 已有 Helm 工作流：[Helm Chart](docs/installation/install-helm.md)
- 想先了解第一次登录和纳管流程：[快速开始](docs/getting-started.md)
- 想看完整说明：[在线文档](https://surrychen.github.io/cylism-manager/)

### 部署脚本

适合单节点控制面或希望交互式初始化的环境：

```bash
git clone https://github.com/SurryChen/cylism-manager.git
cd cylism-manager

bash scripts/deploy-platform.sh \
  --image ghcr.io/surrychen/cylism-manager:latest \
  --namespace cylism-system \
  --ssh-key ~/.ssh/id_ed25519 \
  --verify-image-pull
```

### Helm Chart

适合已经通过 Helm values 管理应用的环境。请先按[Helm 安装指南](docs/installation/install-helm.md)创建 Secret：

```bash
helm upgrade --install cylism-manager charts/cylism-manager \
  --namespace cylism-system \
  --create-namespace \
  --set image.repository=ghcr.io/surrychen/cylism-manager \
  --set image.tag=latest \
  --set secrets.existingSecret=cylism-secret \
  --set ssh.existingSecret=cylism-ssh-key
```

## 几个重要的运行说明

- Manager 的 SQLite 数据保存在 PVC 中。升级前请备份 PVC，正常升级不要删除它。
- SQLite 是单写入数据库，Deployment 使用 `Recreate` 策略，升级时会有短暂不可用。
- Manager 需要较广的 Kubernetes 权限，不适合只允许 Namespace 级只读访问的共享集群。
- K3s 的节点加入和 `vpn-auth` 诊断只有在识别到 K3s 后才会出现；普通 Kubernetes 环境仍可使用通用功能。

## 开发

需要 Go 1.25+、Node.js 24+ 和 npm。根据改动范围，可以额外使用 Docker、Helm、Python 3.10+ 或 Kubernetes 测试集群。

```bash
go test ./...
go run ./cmd/platform

npm --prefix web ci
npm --prefix web run dev
```

完整验证命令和 OpenSpec 变更流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

Cylism Manager 采用 [Apache License 2.0](LICENSE)。
