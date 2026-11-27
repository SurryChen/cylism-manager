# 配置参考

生产环境通过 Kubernetes Secret 和 ConfigMap 注入运行时配置。开发环境可参考仓库中的 `config/config.example.yaml`，但不要把生产配置复制进镜像或 Git 仓库。

## Secret 配置

默认 Secret 名称为 `cylism-secret`，包含：

| 键 | 说明 |
| --- | --- |
| `encryption-key` | 长度必须为 32 字节，用于加密受保护的平台数据。 |
| `jwt-secret` | JWT 签名密钥，应使用随机高熵值。 |
| `admin-password` | 初始管理员密码。 |

服务器 SSH 凭据由服务器记录单独加密保存；纯 Kubernetes/K3s 部署不需要额外的全局 SSH Secret。限制拥有读取凭据和 Secret 权限的主体数量。

## ConfigMap 配置

默认 ConfigMap 名称为 `cylism-config`，常用键如下：

| 键 | 说明 |
| --- | --- |
| `admin-user` | 初始管理员用户名，默认 `admin`。 |
| `public-url` | 告警链接和 GitHub App 授权回跳使用的平台公开 HTTPS 地址。 |
| `access-token-ttl` | 访问令牌有效期，单位秒，默认 `7200`。 |
| `refresh-token-ttl` | 刷新令牌有效期，单位秒，默认 `604800`。 |
| `operation-log-retention-days` | 操作日志保留天数，默认 `30`。 |

更新 ConfigMap 或 Secret 后，重启 Deployment 使新值生效：

```bash
kubectl -n cylism-system rollout restart deployment/cylism-manager
kubectl -n cylism-system rollout status deployment/cylism-manager
```

## 静态资源

在「云服务 → 对象存储 → 静态资源」选择已有的云连接和专用 Bucket。Manager 从云连接读取访问凭据，从 Bucket 信息取得地域。

保持 Bucket 私有，并将 Manager 的 HTTPS 来源加入 Bucket 的 CORS 规则，允许 `PUT` 以及 `content-type`、`x-oss-object-acl`、`x-oss-forbid-overwrite` 请求头。云连接需要在专用桶内上传、读取、复制、删除对象及设置对象 ACL 的权限。

浏览器使用五分钟有效的 OSS 上传签名，把 PNG 或 JPEG 直传到 `pending/`。Manager 通过 OSS 校验大小、类型和图片尺寸，再复制到 `images/` 并让该对象支持公开读。项目只保存匿名读取验证通过的 URL。尚未完成发布的对象保存在私有前缀，可通过 OSS 生命周期规则按需清理。

## 数据持久化与备份

SQLite 默认位于容器内 `/data/cylism.db`，由 PersistentVolumeClaim 提供持久化。备份应在维护窗口内进行，并包含：

1. 对 Manager PVC 做存储卷快照或一致性备份。
2. 记录正在运行的镜像 tag、Helm revision 或部署清单版本。
3. 单独备份 Secret 的管理系统记录，不要将解密后的 Secret 导出到普通文件。

恢复前先停止或缩容 Manager，确认备份与目标版本兼容后恢复 PVC 数据，再重新部署应用。
