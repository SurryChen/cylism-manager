# Cylism Manager 当前部署指南

本文描述当前仓库的部署方式，适用于 Linux 主机、单节点 K3s 和已有 Tailscale tailnet。当前仓库提供两条路径：本地脚本直装，以及 GitHub Release + Helm Chart。它对应仓库中的 `scripts/`、`charts/`、`Dockerfile` 和 `k8s/platform-deployment.yaml` 的历史基线。

## 部署拓扑

- 控制面主机运行 Tailscale、单节点 K3s 和 Cylism Manager。
- Manager 以单副本运行，通过 `/run/tailscale` 访问宿主机 `tailscaled` socket。
- SQLite 和平台运行数据保存在控制面主机 `/data/cylism-manager`，由 Deployment 以 `hostPath` 挂载到容器 `/data`。
- Worker 主机加入同一个 tailnet，再通过平台的服务器/节点流程纳管。

## 前置条件

- Linux 控制面主机，已加入目标 Tailscale tailnet。
- 已安装 Docker（构建镜像）和 SSH 客户端。
- 可访问 K3s 集群的 kubeconfig，或可使用 `sudo k3s kubectl`。
- 私有镜像仓库（如果不使用本地镜像）。
- 本地开发时可参考 `config/config.example.yaml`；生产部署不再要求把 `config/config.yaml` 打进镜像。

## 1. 配置 Tailscale

已有 Tailscale 的主机只需确认服务在线；新主机可使用脚本：

```bash
./scripts/install-tailscale.sh <tskey-auth-...> <hostname>
```

脚本当前使用 `--accept-routes`。如果所在环境不希望 Tailscale 管理 DNS，请在执行后检查并按需补充 `--accept-dns=false`。确认控制面可以访问 tailnet 中的目标主机：

```bash
tailscale status
tailscale ip -4
```

## 2. 初始化 K3s 与平台资源

在控制面主机执行：

```bash
./scripts/init-k3s.sh
```

该脚本会：

1. 在未检测到 `kubectl` 时安装 K3s。
2. 安装 cert-manager（如果 CRD 尚不存在）。
3. 调用 `scripts/deploy-platform.sh`，交互式创建或复用运行时 Secret 并应用 `k8s/platform-deployment.yaml`。
4. 输出 K3s worker join token 和本地端口转发命令。

应用清单前，请检查并按环境修改：

- `spec.template.spec.nodeSelector` 中的控制面节点名称。
- Deployment 的镜像地址和镜像仓库认证。
- `cylism-ssh-key` Secret（平台通过它执行 SSH 纳管）。
- `cylism-secret` 中的 `encryption-key`、`jwt-secret` 和 `admin-password`。推荐直接运行 `scripts/deploy-platform.sh`，脚本会复用已有 Secret，只交互补齐缺失字段。

## 3. 构建和发布镜像

镜像不应包含生产密码。程序支持环境变量覆盖配置文件，生产部署由 Kubernetes Secret/ConfigMap 注入运行时配置：

```bash
bash scripts/deploy-platform.sh --image <registry>/cylism-manager:<tag>
```

脚本会自动创建或复用 `cylism-secret`、`cylism-config` 和 `cylism-ssh-key`，更新 Deployment 镜像并等待 rollout。已有 Secret 中的值不会被轮换。

GitHub tag `v*` 发布后，默认镜像会推送到 GitHub Container Registry：

```text
ghcr.io/surrychen/cylism-manager:v1.2.3
```

仓库内的 `scripts/deploy.sh` 是当前维护者环境的快捷部署脚本，包含固定的镜像仓库、SSH 主机和密钥路径；使用前必须替换这些环境相关变量，不能直接照搬到其他环境。

## 4. Helm 发布包部署

GitHub tag `v*` 发布后，会生成 Helm Chart 包和部署压缩包。安装 Chart 时，默认复用现有 Secret 名称：

```bash
helm upgrade --install cylism-manager \
  oci://ghcr.io/surrychen/charts/cylism-manager \
  --version 1.2.3 \
  --namespace default
```

如需覆盖镜像版本，可以指定：

```bash
helm upgrade --install cylism-manager \
  oci://ghcr.io/surrychen/charts/cylism-manager \
  --version 1.2.3 \
  --set image.tag=v1.2.3
```

如果集群里还没有 `cylism-secret`，可以让 Chart 直接创建，或者先用 `scripts/deploy-platform.sh` 交互式补齐。

## 5. 首次访问与节点纳管

本地开发或首次验证可使用：

```bash
kubectl port-forward svc/cylism-manager 8080:8080
```

浏览器访问 `http://127.0.0.1:8080`，使用配置中的管理员账号登录。之后在平台内：

1. 检查 Tailscale 状态和系统设置。
2. 导入或新增服务器并配置 SSH 凭据。
3. 执行服务器预检/激活。
4. 将已激活服务器加入为 K3s worker。

平台使用 Kubernetes API 读取 Node、Workload、Service、ConfigMap、Secret、Ingress 等实时状态；SQLite 只保存平台元数据、凭据和审计记录。

## 6. 常用检查

```bash
kubectl get pods -l app=cylism-manager
kubectl logs deployment/cylism-manager
kubectl get nodes -o wide
kubectl get crd certificates.cert-manager.io
```

若 Pod 无法启动，优先检查：

- 运行时 Secret 是否存在，且三个安全字段已填写。
- `encryption.key` 是否恰好 32 字节。
- Deployment 的 `nodeSelector` 是否匹配控制面节点。
- `/run/tailscale` 是否挂载了宿主机 socket 目录。
- 镜像仓库凭据和 `cylism-ssh-key` Secret 是否存在。

## 安全注意事项

- 不要提交 `config/config.yaml`、Tailscale Auth Key、SSH 私钥或 Kubernetes Secret YAML。
- 历史中曾经出现过的凭据必须轮换，删除文件不能使旧凭据恢复安全。
- 生产环境应使用私有镜像仓库，并规划将配置从镜像构建阶段迁移为 Kubernetes Secret/挂载注入。
- 不要执行旧迁移文档中的 `rm -rf /var/lib/rancher/k3s/server/tls` 等命令，除非已完成备份并明确需要重建 K3s 证书。

## 相关文件

- `scripts/install-tailscale.sh`：安装并注册 Tailscale。
- `scripts/init-k3s.sh`：初始化 K3s、cert-manager 和平台清单。
- `scripts/deploy.sh`：维护者环境的镜像构建/推送/部署快捷脚本。
- `k8s/platform-deployment.yaml`：平台 Deployment、Service、RBAC 和 hostPath 挂载。
- `docs/archive/operations/k3s-tailscale-ip-migration.md`：旧的特定环境 IP 迁移记录，仅供历史排障参考。
