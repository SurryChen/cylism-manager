<div align="center">

# Cylism Manager

**给个人服务器、Kubernetes 集群和自托管服务用的管理面板。**

[![Build](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml/badge.svg?branch=main)](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml)
[![Docs](https://github.com/SurryChen/cylism-manager/actions/workflows/docs-pages.yml/badge.svg?branch=main)](https://surrychen.github.io/cylism-manager/)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

[English](README.en.md) · [在线文档](https://surrychen.github.io/cylism-manager/) · [快速开始](docs/getting-started.md) · [参与贡献](CONTRIBUTING.md) · [安全说明](SECURITY.md)

</div>

Cylism Manager 是一个用于管理 Linux 服务器、Kubernetes 集群和自托管服务的面板。

<p align="center">
  <a href="docs/assets/screenshots/platform-overview.png">
    <img src="docs/assets/screenshots/platform-overview.png" alt="Cylism Manager 平台健康度概览" width="1200">
  </a>
</p>

## 目前能做什么

- 查看纳管主机、节点、服务和近期操作的状态
- 通过 SSH 做主机预检和基础维护
- 查看和管理常见的 Kubernetes 资源
- 管理镜像、部署记录和应用发布
- 配置服务域名与证书

## 安装

支持部署脚本和 Helm Chart。安装需要一个可访问的 Kubernetes 或 K3s 集群。平台数据默认保存到安装时创建的 PVC；使用 Helm 时，也可以指定 StorageClass 或已有 PVC。

- [部署脚本](docs/installation/install-script.md)：适合交互式初始化
- [Helm Chart](docs/installation/install-helm.md)：适合已有 Helm 工作流的环境
- [快速开始](docs/getting-started.md)：第一次登录和纳管流程
- [在线文档](https://surrychen.github.io/cylism-manager/)：完整说明

### 部署脚本

适合单节点控制面或交互式初始化：

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

请先按 [Helm 安装指南](docs/installation/install-helm.md) 创建 Secret：

```bash
helm upgrade --install cylism-manager charts/cylism-manager \
  --namespace cylism-system \
  --create-namespace \
  --set image.repository=ghcr.io/surrychen/cylism-manager \
  --set image.tag=latest \
  --set secrets.existingSecret=cylism-secret \
  --set ssh.existingSecret=cylism-ssh-key
```

## 运行说明

- Manager 的 SQLite 数据保存在 PVC 中。升级前请备份 PVC，正常升级不要删除它。
- SQLite 是单写入数据库，Deployment 使用 `Recreate` 策略，升级时会有短暂不可用。

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
