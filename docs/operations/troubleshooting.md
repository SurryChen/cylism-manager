# 常见问题

## Pod 无法启动或持续重启

```bash
kubectl -n cylism-system describe pod -l app.kubernetes.io/component=manager
kubectl -n cylism-system logs deployment/cylism-manager --previous
```

依次检查：镜像是否可拉取、`cylism-secret` 是否包含三个必需键、`encryption-key` 是否为 32 字节、`cylism-ssh-key` 是否存在、NodeSelector 是否命中控制面节点，以及 `/run/tailscale` / 数据目录是否可挂载。

## 镜像拉取失败

- 对公开镜像，确认镜像地址和 tag 存在，并从集群节点确认网络可达。
- 对私有镜像，确认 imagePullSecret 位于同一 Namespace，且 ServiceAccount 或 Pod 模板已引用它。
- 使用脚本安装时加 `--verify-image-pull`，让脚本在 rollout 前验证拉取能力。

## 无法登录

确认管理员用户名和密码来自当前运行版本使用的 Secret / ConfigMap。不要在日志、Issue 或聊天中粘贴 Secret 内容。若需要重置密码，按组织的 Secret 轮换流程更新 `admin-password`，再重启 Deployment。

## Tailscale 或服务器预检失败

```bash
tailscale status
tailscale ip -4
```

确认控制面与目标服务器在同一 tailnet，ACL 允许必要的 SSH 和管理流量，目标主机的 SSH 服务运行，且 Manager 使用的私钥与目标用户匹配。优先修复网络与认证问题，再重复预检；不要在未经确认的情况下反复触发加入集群操作。

## Kubernetes 权限拒绝

```bash
kubectl -n cylism-system get serviceaccount,clusterrole,clusterrolebinding
kubectl auth can-i get nodes --as=system:serviceaccount:cylism-system:cylism-manager
```

Helm Release 名称会影响 ServiceAccount 名称。用实际部署生成的名称替换示例中的 `cylism-manager`，并重新渲染 Chart 或检查部署清单，确认绑定的 ClusterRole 未被手动修改。

## 需要更多信息

收集版本号、脱敏后的 Pod 状态、Deployment 事件和相关操作时间。公开 Issue 不应包含 Token、私钥、完整 kubeconfig、真实域名、IP、主机名或未脱敏日志。安全问题请按[安全说明](../security.md)私下报告。
