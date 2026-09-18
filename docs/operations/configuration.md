# 配置参考

生产环境通过 Kubernetes Secret 和 ConfigMap 注入运行时配置。开发环境可参考仓库中的 `config/config.example.yaml`，但不要把生产配置复制进镜像或 Git 仓库。

## Secret 配置

默认 Secret 名称为 `cylism-secret`，包含：

| 键 | 说明 |
| --- | --- |
| `encryption-key` | 长度必须为 32 字节，用于加密受保护的平台数据。 |
| `jwt-secret` | JWT 签名密钥，应使用随机高熵值。 |
| `admin-password` | 初始管理员密码。 |

SSH 私钥单独存放在默认名为 `cylism-ssh-key` 的 Secret 中，并以只读方式挂载到 Manager Pod。限制拥有读取 Secret 权限的主体数量。

## ConfigMap 配置

默认 ConfigMap 名称为 `cylism-config`，常用键如下：

| 键 | 说明 |
| --- | --- |
| `admin-user` | 初始管理员用户名，默认 `admin`。 |
| `public-url` | 告警或外部链接使用的平台公开访问地址。 |
| `access-token-ttl` | 访问令牌有效期，单位秒，默认 `7200`。 |
| `refresh-token-ttl` | 刷新令牌有效期，单位秒，默认 `604800`。 |
| `operation-log-retention-days` | 操作日志保留天数，默认 `30`。 |

更新 ConfigMap 或 Secret 后，重启 Deployment 使新值生效：

```bash
kubectl -n cylism-system rollout restart deployment/cylism-manager
kubectl -n cylism-system rollout status deployment/cylism-manager
```

## 数据持久化与备份

SQLite 默认位于容器内 `/data/cylism.db`，由 PersistentVolumeClaim 提供持久化。备份应在维护窗口内进行，并包含：

1. 对 Manager PVC 做存储卷快照或一致性备份。
2. 记录正在运行的镜像 tag、Helm revision 或部署清单版本。
3. 单独备份 Secret 的管理系统记录，不要将解密后的 Secret 导出到普通文件。

恢复前先停止或缩容 Manager，确认备份与目标版本兼容后恢复 PVC 数据，再重新部署应用。
