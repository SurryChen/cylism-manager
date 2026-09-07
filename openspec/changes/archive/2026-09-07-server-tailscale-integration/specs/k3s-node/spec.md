## MODIFIED Requirements

### Requirement: 节点加入集群
系统 SHALL 将已注册服务器通过 Tailscale 网络加入 K3s 集群，并提供实时进度日志。

#### Scenario: 完整加入流程
- **WHEN** 用户触发服务器加入集群且前置检测全部通过
- **THEN** 系统执行 12 步流程（前置检测 5 步 + Tailscale 安装 3 步 + k3s-agent 安装 4 步），通过 WebSocket 推送每步进度，完成后更新 `k8s_node_name`、`cluster_role`、`tailscale_ip`、`tailscale_online`

#### Scenario: 前置检测未通过
- **WHEN** 前置检测返回 all_pass=false
- **THEN** 系统不继续后续步骤，弹窗展示检测失败项

#### Scenario: Tailscale 已安装跳过安装
- **WHEN** 目标机器已安装 Tailscale
- **THEN** 系统跳过 Tailscale 安装步骤，直接执行 `tailscale up` 注册到网络
