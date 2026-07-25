# k3s-infra-console-reposition 设计文档

> 版本：v1.0  
> 日期：2026 年 7 月 25 日

## 1. 背景

当前项目已经拥有部分 K3s、Service、Config、Ingress 和节点管理能力，但产品结构仍残留 NGINX、acme.sh 和 Agent 中心化设计。为了保证后续实现可以真正落地，需要先统一以下三个问题：

1. 平台到底管理什么资源，什么资源不该落库
2. 页面结构按什么对象模型组织
3. Tailscale 在整套系统里处于什么层次

## 2. 技术决策

### 决策一：V1 固定为单 control-plane 的单集群模型

原因：

- 这是与你描述的部署场景最一致的最小可行范围
- 可显著降低 join token、证书、etcd、失效恢复和 UI 状态建模复杂度
- 能让“服务器 -> 激活 -> 加入 worker -> 资源管理”主链路稳定闭环

替代方案：

- 直接支持多 control-plane / HA K3s

未选原因：

- 超出当前产品必要复杂度
- 会把大部分实现成本耗在控制面生命周期管理，而不是用户当前真正要用的运维能力

### 决策二：K3s 节点统一以 Tailscale IPv4 注册与映射

原因：

- 避免公网 IP、局域网 IP、Tailscale IP 混用
- 让服务器记录和 K8s Node 可以稳定映射
- 适合穿透 NAT 的混合网络场景

替代方案：

- 混用公网/内网地址，Tailscale 仅作 SSH 辅助链路

未选原因：

- 容易出现节点注册地址和控制面访问地址不一致的问题
- 增加证书 SAN、node-ip、排障口径复杂度

### 决策三：平台不负责“创建 tailnet”，只负责“消费已有 tailnet”

原因：

- 这与用户真实部署顺序一致：先建 tailnet，再装 K3s 和平台
- 平台部署在 K3s 内时，访问宿主机 tailscaled socket 本身就需要明确前提
- 将 Tailscale 处理成“发现与补装”能力，比“从零初始化”更稳妥

替代方案：

- 平台首次启动自动安装/初始化控制面宿主机 Tailscale

未选原因：

- 平台已经运行时，控制面宿主机通常已经联网并加入 tailnet
- 该流程容易与用户已有的宿主机 Tailscale 状态冲突

### 决策四：V1 不引入新的远程常驻 Agent，主机级操作统一使用 SSH

原因：

- 现有需求只要求纳管、预检、加入节点、查看资源，不需要额外长驻进程
- 减少远端组件数量，降低部署与维护成本
- 与现有用户“要有 root 或 sudo 权限”这一操作前提一致

替代方案：

- 继续推进 gRPC Agent 架构

未选原因：

- 会显著增加二进制分发、保活、版本升级和心跳状态管理复杂度
- 与当前用户场景相比收益不足

### 决策五：平台数据库只保存元数据，不复制 K8s 运行态

落库对象：

- 服务器台账
- SSH 凭据密文
- 导入结果
- 系统设置
- 操作审计
- 主机事实缓存

实时读取对象：

- Nodes
- Namespaces
- Workloads
- Services
- ConfigMaps
- Secrets
- Ingresses
- Certificates

原因：

- 减少状态漂移
- 简化一致性问题
- 与 Kubernetes 资源管理工具的通行做法一致

### 决策六：路由能力统一建模为 Kubernetes Ingress

原因：

- 这是 K3s 场景里最自然的 HTTP 入口模型
- 能与现有 Ingress 相关实现衔接
- 避免继续背负“宿主机 NGINX 路由配置”和“集群路由配置”的双重语义

替代方案：

- 路由页同时承载宿主机 NGINX 转发与 K8s Ingress

未选原因：

- 产品概念不清晰
- 不同执行平面下的回滚、校验、审计都不同

### 决策七：证书页面改为扩展感知入口

原因：

- 证书能力在 K3s 场景下更自然地依赖 cert-manager 或类似扩展
- 并非所有集群都在 V1 必须启用证书资源管理
- 可以避免把 acme.sh 宿主机流程继续当作默认主链路

替代方案：

- 继续以 acme.sh 为主能力，直接管理宿主机证书

未选原因：

- 与当前 K3s / Ingress 主路径不匹配
- 会继续加重旧产品心智包袱

## 3. 页面结构设计

### 一级导航

- `概览`
- `基础设施`
- `记录与系统`

### 基础设施二级导航

- `服务器`
- `集群节点`
- `工作负载`
- `服务`
- `配置`
- `路由`
- `证书`

### 记录与系统二级导航

- `审计`
- `系统设置`
- `数据管理（内部）`

## 4. 关键数据模型

### Server

关键字段：

- `name`
- `management_address`
- `ssh_port`
- `ssh_user`
- `privilege_mode`
- `ssh_auth_type`
- `tailscale_device_id`
- `tailscale_hostname`
- `tailscale_ipv4`
- `tailscale_online`
- `activation_state`
- `activation_message`

不再把以下字段作为权威事实持久化：

- `cluster_role`
- `k8s_node_name`
- Agent 心跳状态

### ServerFactCache

用于缓存最近一次主机事实采集结果：

- `os_name`
- `arch`
- `cpu_cores`
- `memory_total_mb`
- `disk_total_gb`
- `kernel_version`
- `last_collected_at`

### SystemSetting

关键项：

- `tailscale_auth_key`
- `k3s_join_token`
- `default_ssh_timeout`

## 5. 关键流程

### 服务器导入

1. 从服务器页的导入入口或系统设置中的组网配置区读取 tailscaled peer 列表
2. 排除控制面宿主机自身
3. 生成导入预览
4. 标记可导入、已存在、冲突项
5. 用户确认后导入

### 服务器激活

1. 验证 SSH 登录
2. 验证 root 或 sudo
3. 验证 OS、systemd、磁盘空间
4. 成功后状态置为 `ready`

### Worker 加入

1. 从“已激活且未入集群”的服务器中选择目标
2. 校验 join token 和控制面 Tailscale IPv4
3. 若目标未加入 tailnet，则尝试使用保存的 auth key 补装
4. 安装 k3s agent，并将 `--node-ip` 指向目标 Tailscale IPv4
5. 等待 Node 在 K8s API 中出现并 Ready

## 6. 风险与缓解

### 风险一：平台访问 tailscaled socket 的部署前提不清晰

缓解：

- 在部署清单中固定要求 hostPath 挂载 socket
- 在概览页和组网页提供状态检查与缺失提示

### 风险二：现有代码仍依赖 Agent / NGINX / acme.sh 旧模型

缓解：

- 先在 spec 层明确哪些 requirement 被移除
- 实现阶段分批下线旧页面入口与旧字段依赖

### 风险三：服务器与 Node 映射失败

缓解：

- 统一要求使用 Tailscale IPv4 注册节点
- 将映射失败归类为 `drifted` 并提供人工排查入口

### 风险四：Secret 明文查看带来安全风险

缓解：

- 默认遮蔽
- 明文查看二次确认
- 强制写入审计日志

### 风险五：工作负载页面过重

缓解：

- 使用命名空间筛选而不是单独页面
- V1 仅覆盖 Deployment / StatefulSet / DaemonSet

## 7. 与现有 change 的关系

截至 2026 年 7 月 25 日，仓库中仍存在未归档的 `server-tailscale-integration` change。该 change 里的“平台首次初始化本机 Tailscale”与“独立组网引导页”假设与本设计不完全一致。

本 change 的定位是：

- 作为新的产品与架构总纲
- 后续实现优先遵循本 change 的边界
- 旧 change 中可复用的实现能力（如 SSH probe、precheck、进度推送）可以保留，但产品前提需要按本 change 校正
