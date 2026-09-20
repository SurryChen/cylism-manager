# 安装前置条件

## 基础环境

- 一个可访问的 Kubernetes 或 K3s 集群；执行安装的账户必须能创建 Namespace、Deployment、Service、Secret、ConfigMap、PersistentVolumeClaim、ServiceAccount、ClusterRole 和 ClusterRoleBinding。
- `kubectl` 可访问集群，或控制面存在可用的 `k3s kubectl`。
- 用于拉取平台镜像的镜像仓库访问权限；公开镜像不需要 imagePullSecret。
- 用于纳管其他主机的 SSH 私钥。私钥只能以 Kubernetes Secret 挂载，不得提交到仓库。

## 持久化与权限要求

默认安装会为 SQLite 数据创建 PersistentVolumeClaim：

| 资源 | 用途 | 风险与保护要求 |
| --- | --- | --- |
| PVC | SQLite 数据库、平台状态 | 需要可用的默认 StorageClass，或在 Helm values 中指定 StorageClass / 已有 Claim；纳入存储卷备份策略。 |

Manager 使用 Kubernetes ClusterRole 管理节点、工作负载、Service、Secret、ConfigMap、证书及相关资源，并可能通过 SSH 连接受管服务器。请限制可创建 Pod、读取 Secret 和修改平台 Deployment 的人员与自动化身份。

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
| Python | 仅在运行项目中的辅助脚本时需要 |

如果要在本地预览或构建这套文档，只需要 Node.js 24+ 和 npm：

```bash
npm ci --prefix docs
npm run dev --prefix docs
```

继续前，请选择[脚本安装](install-script.md)或[Helm 安装](install-helm.md)。
