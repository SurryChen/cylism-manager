# 日常运维

## 平台健康检查

```bash
kubectl -n cylism-system get deployment,pods,svc
kubectl -n cylism-system logs deployment/cylism-manager --tail=200
kubectl get nodes -o wide
```

重点关注 Pod 重启次数、Readiness 状态、镜像拉取错误、权限拒绝和 Tailscale socket 挂载失败。

## 变更前检查

在执行平台升级、节点加入、集群配置下发、证书删除或镜像仓库删除前：

1. 确认操作目标和 Namespace。
2. 检查操作历史和审计日志中的近期失败。
3. 为控制面数据目录和当前 Kubernetes 资源保留可恢复备份。
4. 对生产集群选择维护窗口，并记录当前版本与回退路径。

## 升级策略

- 脚本安装：使用新镜像 tag 重跑部署脚本，保留它创建的部署前备份。
- Helm 安装：更新 values 中的镜像 tag，执行 `helm upgrade`，再用 `helm history` 确认 revision。
- 任何升级完成后：检查 rollout、登录、Tailscale 状态、至少一个服务器预检和必要的 Kubernetes 资源读取。

## 日志与审计

Kubernetes 日志用于诊断运行时问题；平台审计日志用于追溯界面和 API 发起的管理操作。二者都可能包含主机、资源或失败上下文，收集并外发前应按组织安全政策脱敏。
