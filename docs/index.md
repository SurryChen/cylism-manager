# Cylism Manager

Cylism Manager 是面向 **Tailscale + K3s** 场景的基础设施控制台。它把服务器纳管、SSH 主机操作、Kubernetes 资源管理、制品库、证书和审计记录收敛到一个界面。

> 这不是只读仪表盘。部署后的 Manager 需要访问 Kubernetes API、控制面宿主机的 Tailscale socket，并使用受保护的 SSH 凭据操作受管主机。请先阅读[前置条件](installation/prerequisites.md)和[安全说明](security.md)。

## 适用场景

- 控制面节点已加入 Tailscale tailnet，计划纳管同一 tailnet 中的 Linux 主机。
- 使用 K3s 或兼容 Kubernetes 集群，且能够接受平台以 ClusterRole 管理受控资源。
- 希望在同一平台查看服务器、节点、Kubernetes 资源、应用、证书、镜像仓库与审计记录。

## 不适用场景

- 需要严格多租户隔离或只授予 Namespace 级只读权限的共享集群。
- 没有可保护 SSH 私钥、Kubernetes Secret 或控制面宿主机数据目录的环境。
- 希望通过 Docker Compose 获得受支持的生产安装路径。当前公开支持的路径仅为 K3s 部署脚本和 Helm Chart。

## 选择安装方式

| 场景 | 推荐方式 | 入口 |
| --- | --- | --- |
| 单节点控制面或希望交互式初始化 Secret | 部署脚本 | [脚本安装](installation/install-script.md) |
| 已有 Helm 流程或需要声明式 values | Helm Chart | [Helm 安装](installation/install-helm.md) |
| 贡献代码或验证修改 | 本地开发 | [开发与贡献](development/contributing.md) |

## 首次成功标准

完成安装后，应能执行以下检查：

```bash
kubectl get pods -l app.kubernetes.io/component=manager
kubectl get svc
kubectl port-forward svc/cylism-manager 8080:8080
```

然后在浏览器打开 `http://127.0.0.1:8080`，使用部署时设置的管理员账户登录。接下来按照[快速开始](getting-started.md)确认 Tailscale 状态并纳管第一台服务器。

## 产品截图

截图会在经过脱敏审查后加入本页。
