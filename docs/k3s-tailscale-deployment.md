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

手工安装 Tailscale 后，确认控制面可以访问 tailnet 中的目标主机：

```bash
tailscale status
tailscale ip -4
```

## 2. 初始化 K3s 与平台资源

K3s、cert-manager 和平台清单请按当前环境手工安装，再使用 `scripts/deploy-platform.sh` 部署平台。

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

如果 GHCR 镜像是 Public，不需要配置镜像拉取账号密码。如果镜像是 Private，可以让脚本交互式创建或复用 imagePullSecret：

```bash
bash scripts/deploy-platform.sh \
  --image ghcr.io/surrychen/cylism-manager:v1.2.3 \
  --configure-ghcr-pull
```

其中 GitHub Token 需要具备 `read:packages` 权限。脚本只会把它写入 Kubernetes Secret，不会写入仓库文件。

## 4. 测试环境 dev 分支自动更新镜像

测试环境可以使用 `dev` 分支自动部署。该流程只更新平台 Deployment 的镜像，不会自动应用 `k8s/platform-deployment.yaml`、RBAC、Service 或 Helm Chart 变更；这些清单变更仍需通过部署脚本或 Helm 手动升级。

当前 GitHub Actions 的 dev 链路为：

```text
push dev
  → go test / go build / helm lint
  → 构建并推送 GHCR 镜像
  → 签名调用测试环境 /api/platform/deployments
  → 平台自更新 Deployment 镜像
```

dev 镜像会推送以下 tag：

```text
ghcr.io/surrychen/cylism-manager:dev
ghcr.io/surrychen/cylism-manager:dev-<short-sha>
ghcr.io/surrychen/cylism-manager:<full-sha>
```

平台 Webhook 使用不可变的 `dev-<short-sha>` 镜像，方便定位测试环境当前运行的提交。

启用前需要完成三件事：

1. 测试环境 Deployment 已配置 GHCR 拉取权限，例如 `ghcr-pull-secret`。
2. 平台允许的镜像前缀包含：

   ```text
   ghcr.io/surrychen/cylism-manager
   ```

   新版本默认使用该前缀；如果旧数据库里保存过旧镜像仓库前缀，需要在平台发布设置中更新一次。

3. 在平台生成部署 Webhook Secret，并写入 GitHub 仓库 Secrets：

   ```text
   CYLISM_DEV_DEPLOY_URL=https://你的测试环境域名/api/platform/deployments
   CYLISM_DEV_DEPLOY_SECRET=平台生成的部署 Webhook Secret
   CYLISM_DEV_DEPLOY_RESOLVE_IP=可选，GitHub Actions 访问该域名时强制解析到的 IP
   ```

   如果测试环境域名在 GitHub Actions 侧 DNS 不稳定，或者你想固定打到某个内网/公网入口，可以设置 `CYLISM_DEV_DEPLOY_RESOLVE_IP`。工作流会在发起请求时使用 `curl --resolve` 将该域名指向指定 IP，但仍保留原始域名用于 TLS/SNI 校验。

配置完成后，推送 `dev` 分支即可触发测试环境镜像更新：

```bash
git push origin dev
```

## 5. Helm 发布包部署

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

## 6. 首次访问与节点纳管

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

## 7. 常用检查

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

- `k8s/platform-deployment.yaml`：平台 Deployment、Service、RBAC 和 hostPath 挂载。
- `docs/archive/operations/k3s-tailscale-ip-migration.md`：旧的特定环境 IP 迁移记录，仅供历史排障参考。
