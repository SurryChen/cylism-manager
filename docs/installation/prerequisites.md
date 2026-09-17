# 安装前置条件

## 基础环境

- 一台 Linux 控制面主机，已加入目标 Tailscale tailnet。
- 一个可访问的 K3s 或兼容 Kubernetes 集群；执行安装的账户必须能创建 Namespace、Deployment、Service、Secret、ConfigMap、ServiceAccount、ClusterRole 和 ClusterRoleBinding。
- `kubectl` 可访问集群，或控制面存在可用的 `k3s kubectl`。
- 用于拉取平台镜像的镜像仓库访问权限；公开镜像不需要 imagePullSecret。
- 用于纳管其他主机的 SSH 私钥。私钥只能以 Kubernetes Secret 挂载，不得提交到仓库。

## 控制面节点要求

默认安装会把以下宿主机目录挂载到 Manager Pod：

| 目录 | 用途 | 风险与保护要求 |
| --- | --- | --- |
| `/data/cylism-manager` | SQLite 数据库、平台状态 | 纳入主机备份；限制节点与文件系统访问权限。 |
| `/run/tailscale` | `tailscaled` socket | 可代表该节点调用 Tailscale 本地 API；仅允许可信 Manager Pod 挂载。 |

Manager 还使用 Kubernetes ClusterRole 管理节点、工作负载、Service、Secret、ConfigMap、证书及相关资源，并可能通过 SSH 连接受管服务器。请将它部署在受信任的控制面节点，避免与不受信任的工作负载共用节点访问权限。

## Kubernetes 依赖

- K3s 或 Kubernetes 集群处于健康状态。
- 如需证书功能，集群中应安装 `cert-manager`；部署脚本会在缺失时询问是否安装。
- 如需使用 DNS Webhook 或自定义证书签发器，请先按组织的证书和 DNS 规范配置它们。

## 本地开发依赖

| 工具 | 版本 |
| --- | --- |
| Go | 1.25+ |
| Node.js | 24+（CI 基线） |
| npm | 与 Node.js 匹配的版本 |
| Docker | 可选，用于镜像构建验证 |
| Helm | 可选，用于 Chart 验证与安装 |
| Python | 3.10+，仅用于构建文档站 |

继续前，请选择[脚本安装](install-script.md)或[Helm 安装](install-helm.md)。
