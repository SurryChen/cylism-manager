# Helm 安装

Helm Chart 位于 `charts/cylism-manager`。它适合已经通过 values 管理集群应用的环境。

## 1. 准备 Secret

Chart 默认引用 `cylism-secret` 和 `cylism-ssh-key`。建议先在目标 Namespace 创建这两个 Secret：

```bash
kubectl create namespace cylism-system

kubectl -n cylism-system create secret generic cylism-secret \
  --from-literal=encryption-key='<32-byte-encryption-key>' \
  --from-literal=jwt-secret='<random-jwt-signing-secret>' \
  --from-literal=admin-password='<initial-admin-password>'

kubectl -n cylism-system create secret generic cylism-ssh-key \
  --from-file=id_ed25519=/secure/path/to/id_ed25519
```

`encryption-key` 必须恰好为 32 字节。请通过密码管理器或组织的 Secret 管理流程生成这些值；不要将它们写进 `values.yaml`、Shell 历史或仓库文件。

## 2. 安装本地 Chart

```bash
helm upgrade --install cylism-manager charts/cylism-manager \
  --namespace cylism-system \
  --set image.repository=ghcr.io/<owner>/cylism-manager \
  --set image.tag=<version> \
  --set secrets.existingSecret=cylism-secret \
  --set ssh.existingSecret=cylism-ssh-key
```

如果通过 OCI Registry 安装 Release Chart，将 `charts/cylism-manager` 替换为项目 Release 中给出的 OCI Chart 地址和版本。

## 3. 使用受控 values 文件

不含密钥的 values 文件可以纳入受控配置仓库：

```yaml
image:
  repository: ghcr.io/<owner>/cylism-manager
  tag: <version>

data:
  storageClass: <storage-class> # 留空时使用集群默认 StorageClass
  size: 5Gi
  # 已有 PVC 时设置 existingClaim，Chart 不会新建 PVC。
  # existingClaim: cylism-manager-data

ssh:
  existingSecret: cylism-ssh-key

secrets:
  existingSecret: cylism-secret

config:
  adminUser: admin
  publicURL: https://manager.example.com
```

然后执行：

```bash
helm upgrade --install cylism-manager charts/cylism-manager \
  --namespace cylism-system \
  --values values.yaml
```

## 验证、升级和回退

```bash
helm -n cylism-system status cylism-manager
kubectl -n cylism-system rollout status deployment/cylism-manager
helm -n cylism-system history cylism-manager
```

升级时更新镜像 tag 后再次执行 `helm upgrade --install`。如需回退，先查看历史，再选择已验证的 revision：

```bash
helm -n cylism-system rollback cylism-manager <revision>
```

回退 Chart 不会自动回退 SQLite 数据。升级前应备份 Manager PVC 中的数据，并在现有 hostPath 安装迁移前将数据库复制到新 PVC。

Manager 使用 SQLite 单写入数据库，Chart 默认采用 `Recreate` 更新策略：升级时会先停止旧 Pod，再启动新 Pod，避免两个版本分别写入不同的数据卷。请为升级安排短暂维护窗口。
