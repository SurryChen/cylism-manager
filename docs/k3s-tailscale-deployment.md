# K3s 兼容部署说明

本文档是 Release 部署包中的 K3s 兼容说明。Cylism Manager 的基线是 Kubernetes 兼容 API，不要求安装或接入 Tailscale。

公开安装入口：

- [安装前置条件](installation/prerequisites.md)
- [脚本安装](installation/install-script.md)
- [Helm 安装](installation/install-helm.md)
- [配置参考](operations/configuration.md)
- [常见问题](operations/troubleshooting.md)

## Kubernetes 基线

- Manager 通过 Kubernetes API 管理集群资源，并以 PVC 持久化 SQLite 数据。
- 部署清单不挂载宿主机 socket，不固定到特定节点；请按实际存储、调度和安全策略提供 values 或补丁。
- 服务器由操作员配置可达 SSH 地址和凭据。该地址可以由私有网络、DNS 或独立维护的网络工具提供。

## K3s 可选优化

平台在集群版本中识别到 `+k3s` 标记后才启用 K3s 专属能力：

1. K3s 工作节点加入保留 SSH 预检和 `k3s-agent` 安装，使用操作员已配置的控制面可达地址与 K3s Join Token。
2. 网络诊断只读取活动 `k3s` 或 `k3s-agent` 单元的 `vpn-auth` 配置标记，并展示不含凭据的兼容状态。

平台不会安装、认证、注册或升级任何外部 VPN。若环境使用 Tailscale 或其他网络工具，请在平台外维护它们的节点、凭据和访问策略；Manager 只使用已保存的 SSH 管理地址。

## 升级注意事项

从旧版本升级时：

1. 将 SQLite 数据从旧的宿主机目录迁移到 Manager PVC，并在维护窗口内完成备份与恢复验证。
2. 删除旧 Helm values 中的 `tailscale.hostPath` 和 `data.hostPath` 配置。
3. 启动新版本后，平台会删除旧的 `tailscale_auth_key` 配置；该值不会导出或记录到日志。
4. 旧的 `/api/tailscale/*` 主机管理接口已移除。依赖这些接口的自动化需要改为直接调用所属网络系统。

与历史环境有关的迁移记录保留在 `docs/archive/`，仅用于回溯，不作为当前部署指南。
