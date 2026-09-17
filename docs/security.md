# 安全说明

## 权限边界

Cylism Manager 旨在执行真实的基础设施管理动作，因此需要高权限：

- Kubernetes ClusterRole 可读取和管理多类集群资源。
- 控制面宿主机的 Tailscale socket 允许平台读取 tailnet 状态并发起本地 Tailscale API 调用。
- SSH 私钥允许平台连接已配置的受管服务器。
- SQLite 数据库存储平台元数据、审计信息和受保护的配置数据。

只应在受信任的控制面节点和受控 Kubernetes 集群中部署它。限制可创建 Pod、读取 Secret、访问控制面文件系统和修改平台 Deployment 的人员与自动化身份。

## Secret 管理

- 使用 Kubernetes Secret 或外部 Secret 管理系统保存加密密钥、JWT 密钥、管理员密码、SSH 私钥和镜像拉取凭据。
- 不要提交 `.env`、`config/config.yaml`、私钥、kubeconfig、Token 或 Secret YAML。
- 不要在 GitHub Issue、Pull Request、聊天记录或截图中粘贴敏感值。
- 一旦凭据进入 Git 历史、日志或公开渠道，应立即在原系统中轮换；删除文件不足以使凭据重新安全。

## 漏洞报告

请不要通过公开 Issue 报告可利用漏洞或粘贴复现所需的凭据。优先使用仓库的 GitHub Private Vulnerability Reporting 功能创建私密安全通报；若该功能尚未启用，请联系仓库维护者并仅分享最小必要的脱敏信息。

报告应包含受影响版本、影响说明、最小复现步骤和建议修复方向。维护者会确认接收、评估影响并在修复后协调披露。
