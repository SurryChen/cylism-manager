# 脚本安装

部署脚本适合单节点控制面和首次安装。它会检测 `kubectl` / `k3s kubectl`，创建目标 Namespace，备份已有平台资源，并在缺失时交互补齐平台 Secret。已有 Secret 不会被脚本自动轮换。

## 1. 获取源码或 Release 部署包

从源码仓库执行：

```bash
git clone https://github.com/<owner>/cylism-manager.git
cd cylism-manager
```

也可以使用 Release 提供的部署包。确认包内包含 `scripts/deploy-platform.sh` 和 `k8s/platform-deployment.yaml`。

## 2. 准备镜像

确定一个可由集群节点拉取的镜像，例如：

```text
ghcr.io/<owner>/cylism-manager:<version>
```

## 3. 执行部署

```bash
bash scripts/deploy-platform.sh
```

脚本默认使用 `ghcr.io/surrychen/cylism-manager:latest` 和 `cylism-system`，运行时会交互询问 HTTPS IP。输入 IP 后，脚本会检查 K3s Traefik 的 443 端口，并创建 30 天有效的临时自签证书和 HTTPS Ingress。省略 IP 会跳过公网入口。

首次运行时，脚本会：

1. 检查 Kubernetes 访问方式；当本机未安装 K3s 时会先征询是否安装。
2. 在目标 Namespace 创建或复用部署资源。
3. 备份已有 Deployment、Service、RBAC、ConfigMap 和 Secret 键名到本地受限目录；不会导出 Secret 值。
4. 创建或复用 `cylism-secret` 和 `cylism-config`。
5. 询问或生成 `encryption-key`、`jwt-secret` 和管理员密码，并等待 Deployment rollout 完成。

卸载部署资源（默认保留 PVC 和 `cylism-secret`）：

```bash
bash scripts/deploy-platform.sh uninstall
```

需要连同数据库 PVC 和 `cylism-secret` 一起删除时，显式使用 `--purge-data`：

```bash
bash scripts/deploy-platform.sh uninstall --purge-data
```

## 常用参数

```bash
bash scripts/deploy-platform.sh --help
```

| 参数 | 用途 |
| --- | --- |
| `--image` | 指定公开平台镜像。 |
| `--namespace` | 目标 Namespace，默认 `cylism-system`。 |
| `--node` | 新安装时指定控制面节点选择器。 |
| `--https-ip` | 为指定 IPv4 地址创建临时自签 HTTPS 入口。 |
| `--verify-image-pull` | 部署前创建临时 Pod 验证公开镜像可拉取。 |
| `--backup-dir` | 指定部署前备份目录。 |
| `--skip-backup` | 跳过部署前备份；仅在已有可靠备份时使用。 |
| `--purge-data` | 卸载时同时删除 PVC 和 `cylism-secret`，默认不删除。 |
| `--yes` | 卸载时跳过确认。 |

## 验证与升级

```bash
kubectl -n cylism-system rollout status deployment/cylism-manager
kubectl -n cylism-system get pods,svc
kubectl -n cylism-system logs deployment/cylism-manager
```

升级时使用同一命令并更换 `--image` tag。脚本会复用已有 Secret；升级前的资源备份可用于人工比对和回退。若 rollout 失败，请阅读[常见问题](../operations/troubleshooting.md)。
