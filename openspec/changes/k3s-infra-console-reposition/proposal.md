## Why

截至 2026 年 7 月 25 日，仓库中的产品叙事、能力边界和页面结构仍混合了三套心智：

- 旧的宿主机 NGINX + acme.sh 管理面板
- 过渡期的 gRPC Agent 架构
- 新增中的 K3s / Tailscale 运维能力

这导致以下问题：

- “服务器”页面同时承担台账、Agent 状态和集群节点职责，概念混杂
- 路由、证书、站点等能力仍以 NGINX 时代的对象模型为核心，不符合当前 K3s 使用场景
- Tailscale 在部分设计里被当作需要由平台从零初始化的系统，但真实部署顺序是用户先完成 tailnet，再部署平台
- 现有 spec 仍把 Agent 部署、心跳状态和 acme.sh 证书流程视为主路径，与目标产品方向冲突

需要一次产品与架构层面的重定位，把 Cylism Manager 收敛为“面向 Tailscale 场景的单控制面 K3s 基础设施控制台”，并为后续实现提供可审查的能力基线。

## What Changes

- 将产品定位从“Nginx/证书管理面板”重写为“Tailscale-aware 的单集群 K3s 运维面板”
- 明确 V1 范围：
  - 单个 tailnet
  - 单个 K3s 集群
  - 单个 control-plane
  - 多个 worker
- 明确架构边界：
  - Tailscale 负责网络与发现
  - SSH 负责主机级操作
  - Kubernetes API 负责集群运行态事实来源
  - SQLite 负责元数据、凭据、审计和缓存
- 将“服务器”和“集群节点”拆成两个独立页面与能力模型
- 将命名空间收敛为工作负载、服务、配置和路由的统一筛选维度，而不是一级页面
- 将“路由”固定建模为 Kubernetes Ingress 管理，而不是宿主机反向代理
- 将“证书”从 acme.sh 主链路改为扩展感知页面，优先支持 cert-manager 场景
- 将 Tailscale 相关能力改为“服务器导入 + 系统设置中的组网配置”，而非基础设施中的独立引导页面
- 更新 OpenSpec capability，使其与新的产品边界一致

## Capabilities

### New Capabilities

- `tailnet-discovery`: 读取本机 tailscaled socket、预览 tailnet peer、检测冲突并导入服务器

### Modified Capabilities

- `server-management`: 从 Agent 管理转为服务器台账、SSH 激活、主机事实与实时资源查看
- `k3s-node`: 节点信息以 K8s API 为准，只支持从已激活服务器加入 worker
- `ui-navigation`: 导航重构为概览 / 基础设施 / 记录与系统三层结构
- `dashboard-audit`: 仪表盘增加组网与集群引导信息，审计保持为高风险操作主线
- `cert-management`: 从 acme.sh 生命周期改为扩展感知的证书资源入口

## Non-goals

- 本次变更不实现多集群管理
- 本次变更不支持通过 UI 新增第二个 control-plane
- 本次变更不恢复或扩展 gRPC Agent 架构
- 本次变更不继续以宿主机 NGINX 站点对象作为核心业务模型
- 本次变更不把平台做成持续采样的监控系统
- 本次变更不直接归档旧 change；旧 change 的实现取舍将在后续实现阶段逐项清理

## Impact

- `PRODUCT.md`、设计文档和 OpenSpec 能力基线将发生重写
- 现有前端导航与页面组织需要重构：
  - `/servers` 回归服务器台账，并吸收原组网引导入口
  - 新增 `/cluster`
  - `db-admin` 从主路径降级
- 现有后端模型边界需要调整：
  - 服务器元数据与 K8s 运行态分治
  - 去除对 Agent 心跳与部署状态的依赖
- 证书与路由相关实现需要从“宿主机/站点”思路迁移到“集群资源/扩展”思路
