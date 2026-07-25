# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

小团队运维 / DevOps 工程师。用户先用 Tailscale 把多台 Linux 机器组到同一个 tailnet，再选择其中一台部署单节点 K3s 和本平台，通过 Web UI 统一完成服务器纳管、工作节点加入、Kubernetes 资源管理与运维审计。

## Product Purpose

Cylism Manager 是一个面向 Tailscale 场景的 K3s 基础设施控制台。它把“服务器台账 + SSH 纳管 + K3s 集群操作 + Kubernetes 资源可视化与变更”收敛到同一个界面，降低多机器混合网络环境下的运维复杂度。

## Positioning

Tailscale-aware 的单集群 K3s 运维面板：
- Tailscale 负责组网与节点寻址
- SSH 负责远程安装与主机级探测
- Kubernetes API 负责集群内资源的实时事实来源
- 平台数据库仅保存元数据、凭据、审计和缓存

## Operating Context

- 所有候选机器均为 Linux，并加入同一个 Tailscale tailnet
- 用户先选定一台机器作为单控制面节点，部署单节点 K3s
- 平台以单副本方式部署在该 K3s 集群中，并固定运行在控制面宿主机
- 平台通过宿主机 `tailscaled` socket 发现 tailnet 设备
- 平台通过 SSH 或 `sudo` 权限远程管理其他服务器
- 集群节点、工作负载、Service、ConfigMap、Secret、Ingress 等状态直接来自 K8s API，不以数据库为权威

## Capabilities and Constraints

**能力：**
- 基于 Tailscale 发现并导入服务器
- 维护服务器记录、SSH 凭据、激活状态和主机事实信息
- 从已激活服务器中选择节点，自动加入当前 K3s 集群
- 管理工作负载、服务、配置、路由和证书等 Kubernetes 运维对象
- 在证书页内感知扩展状态，而不是为扩展单独提供一级基础设施页面
- 将命名空间作为工作负载、服务、配置与路由的统一筛选维度
- 记录关键变更的审计日志

**约束：**
- 后端 Go + Gin + GORM + SQLite，前端 Vue 3 + Vite
- V1 仅支持一个 K3s 集群、一个控制面节点、多个 worker 节点
- V1 不支持多集群管理，不支持通过 UI 新建第二个 control-plane
- V1 不再以宿主机 NGINX 站点管理为核心能力，路由管理统一基于 Kubernetes Ingress 模型
- V1 仅支持 Linux 服务器，不支持 Windows 节点
- 由于使用 SQLite 和宿主机 socket，平台运行模式为单副本

## Brand Commitments

产品名称仍为 Cylism Manager。产品表达从“NGINX/证书管理面板”调整为“Tailscale + K3s 基础设施控制台”。

## Evidence on Hand

- 当前代码仓库的 K3s、Ingress、Config、Workload、Service、Server 相关实现
- 设计文档：[docs/design/cylism-manager-k3s-infra-console.md](/Users/dxm/MyApp/cylism-manager/docs/design/cylism-manager-k3s-infra-console.md)
- 历史 OpenSpec 变更可作为实现参考，但不再代表最新产品定位

## Product Principles

1. **Tailscale 只负责连通，不负责承载业务模型**  
   大多数业务对象不依附于 Tailscale 存在；Tailscale 主要承担节点发现、地址统一和控制面到节点的通信前提。

2. **平台元数据与集群实时状态分治**  
   服务器凭据、导入记录、审计日志进数据库；节点、工作负载、服务、配置等运行态数据直接来自 K8s API。

3. **先把单控制面场景做扎实**  
   V1 只解决真实可跑通的单控制面 + 多 worker 运维闭环，不为了“未来 HA”提前引入高复杂度。

4. **所有高风险操作都可审计**  
   包括凭据变更、节点加入、节点移除、Secret 明文查看和路由变更。

## Accessibility & Inclusion

无额外无障碍要求已确认。默认遵循 Web 标准可访问性，并优先保证桌面端与移动端都能完成基础运维操作。
