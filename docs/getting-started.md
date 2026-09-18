# 快速开始

完成安装后，按下面顺序验证平台和纳管首台服务器。

## 1. 首次登录

通过 port-forward 或已配置的 Ingress 访问 Manager：

```bash
kubectl -n cylism-system port-forward svc/cylism-manager 8080:8080
```

打开 `http://127.0.0.1:8080`，用部署阶段设置的管理员账户登录。首次登录后应立即将初始密码替换为由组织密码管理器保管的高强度值。

## 2. 检查平台依赖

在系统设置和概览页确认：

- Kubernetes 节点状态正常，集群页面能显示已识别的平台和版本。
- Manager Pod 可以通过 SSH 访问目标主机的操作员配置地址。
- 若计划使用证书，cert-manager 与对应 Issuer 已就绪。
- Manager Pod 没有持续重启或权限错误。

```bash
kubectl -n cylism-system get pods
kubectl -n cylism-system logs deployment/cylism-manager --tail=100
```

## 3. 纳管第一台服务器

1. 确保目标 Linux 主机允许 Manager Pod 通过 SSH 登录。
2. 在“服务器”页面新增主机，填写由操作员维护的可达地址。
3. 为该主机选择最小权限的 SSH 用户和受保护的 SSH 凭据。
4. 运行预检，处理网络、系统依赖和 SSH 权限错误。
5. 确认预检成功后再执行激活或加入集群操作；工作节点加入仅在已识别的 K3s 集群中可用。

平台会记录操作历史和审计记录。对生产主机执行加入、重启、删除或配置下发前，请在界面中复核目标名称和影响范围。

## 4. 下一步

- 配置 SQLite、JWT、令牌有效期和公开访问地址：[配置参考](operations/configuration.md)
- 了解主机、集群和应用的日常检查：[日常运维](operations/operations.md)
- 遇到部署或登录问题：[常见问题](operations/troubleshooting.md)
