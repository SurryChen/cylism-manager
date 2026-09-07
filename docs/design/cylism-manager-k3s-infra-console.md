# Cylism Manager 设计文档

> 主题：Tailscale + 单控制面 K3s 基础设施控制台  
> 版本：v2.1
> 原始设计：2026-07-25；当前实现校准：2026-09-07

> 本文是产品与架构基线，不是逐项 API 参考。实现细节以当前路由、Service、Kubernetes 清单和部署脚本为准。

## 当前实现校准（2026-09-07）

以下内容已根据当前代码补充或修正：

- 平台保留可选的 gRPC/HTTP Agent 能力，用于运行时诊断、受控操作、审批和 Nanobot Runtime；主机纳管仍以 SSH 为主，因此 Agent 不是平台启动前提。
- 路由能力同时覆盖标准 Kubernetes `Ingress` 和 Traefik `IngressRoute`，分别对应 `/api/k8s/ingresses` 与 `/api/routes`。
- 应用发布支持 `ClusterIP`、`NodePort`、`LoadBalancer`，并支持多端口及 TCP/UDP 协议；“不支持四层路由”仅适用于独立路由编排页面，不适用于应用 Service。
- 平台 Deployment 当前使用 `/data/cylism-manager` hostPath 保存 SQLite 和运行数据，并通过 `/run/tailscale` hostPath 访问宿主机 socket；不是 PVC 部署模板。
- 当前已实现 VictoriaMetrics、Alertmanager、Loki、cert-manager、Registry/Registry Proxy 等扩展能力，扩展状态由系统与对应领域页面展示。
- Tailscale 既可由部署前脚本配置，也可通过平台 `/api/tailscale/init` 和 `/api/tailscale/install-script` 管理本机接入；两种方式都不改变“平台依赖已有 tailnet”的产品定位。
- 前端当前以 `/resources`、`/network`、`/cluster` 等聚合页承载资源导航；`/workloads`、`/services`、`/configs`、`/routes`、`/certs` 主要作为兼容入口或重定向。

## 1. 设计目标

本文档把 Cylism Manager 从“宿主机 NGINX/证书管理面板”重构为一套真实可落地的产品方案，服务于以下场景：

1. 用户先用 Tailscale 将多台 Linux 机器组到同一个 tailnet。
2. 用户选择其中一台机器部署单节点 K3s，并把本项目部署到这个集群中。
3. 用户通过本平台统一管理服务器、工作节点加入、Kubernetes 资源、路由与扩展能力。

本设计的核心目标不是“支持所有可能场景”，而是把一个高频、稳定、能端到端跑通的主场景收敛清楚。

## 2. 最终范围

### 2.1 V1 支持

- 单个 Tailscale tailnet
- 单个 K3s 集群
- 单个控制面节点
- 多个 worker 节点
- Linux 主机
- 服务器导入、纳管、SSH 激活
- Worker 节点加入与移除
- Namespace、Workload、Service、ConfigMap、Secret、Ingress/IngressRoute 管理
- 证书能力入口与扩展状态提示
- 审计日志

### 2.2 V1 明确不支持

- 多集群管理
- 多 control-plane / HA K3s 管理
- Windows 节点
- 宿主机 NGINX 站点配置管理
- 持续型主机监控系统替代品
- 无 Tailscale 场景下的集群引导

应用 Service 已支持 `LoadBalancer` 以及多端口 TCP/UDP；这里不提供的是独立于应用 Service 的通用四层路由编排。

## 3. 必须锁定的产品决策

这是整套方案能否“完全可行”的关键。

### 3.1 K3s 节点统一使用 Tailscale IPv4 注册

- 控制面 `advertise-address` 使用控制面机器的 Tailscale IPv4
- Worker `--node-ip` 使用各自的 Tailscale IPv4
- 平台用 `server.tailscale_ipv4 == node.InternalIP` 建立服务器与 K8s 节点映射

这样做的结果是：

- 节点间通信口径统一
- 不混用公网 IP / 内网 IP / Tailscale IP
- 不需要把节点信息落库也能稳定建立映射

### 3.2 路由页管理的是 Kubernetes Ingress，不是宿主机 NGINX

“路由”页统一建模为 HTTP/HTTPS 入口规则：

- 域名
- 路径
- 后端 Service
- 端口
- TLS Secret 引用
- 常见注解

标准 Ingress 是应用和通用 HTTP 路由的主模型；已有 Traefik `IngressRoute` 资源通过独立的高级入口读取和维护。两者都不再表示宿主机 NGINX 站点配置。

### 3.3 集群运行态以 Kubernetes API 为事实来源

以下对象不以数据库为权威：

- Nodes
- Namespaces
- Deployments / StatefulSets / DaemonSets
- Services
- ConfigMaps
- Secrets
- Ingresses
- Certificates（如启用扩展）

数据库只保存平台元数据，不复制整套集群状态。

### 3.4 平台不负责“组织 tailnet”，主要负责“利用已有 tailnet”

用户在部署平台前，已经完成：

- 控制面宿主机加入 tailnet
- 其他候选服务器加入 tailnet，或至少具备后续加入 tailnet 的前提

平台的默认部署前提是 tailnet 已可用；在此前提下平台也提供受控的本机初始化入口：

- 读取本机 `tailscaled` socket，发现 tailnet 设备
- 允许用户保存可复用 auth key，便于远程补装 Tailscale
- 通过 `/api/tailscale/init` 执行本机安装/注册，通过 `/api/tailscale/install-script` 返回脱敏命令
- 用 Tailscale 地址做 SSH 与 K3s 节点加入

平台不会替用户创建 tailnet、管理 tailnet ACL 或替代 Tailscale 控制台；本机初始化属于部署辅助能力。

### 3.5 V1 只支持通过 UI 加入 worker，不支持新增 control-plane

这是最重要的降复杂度策略之一。  
单控制面场景已经足够覆盖大量真实需求，而且可显著降低：

- token / cert / embedded etcd 复杂度
- control-plane 升级与失效恢复复杂度
- 节点状态机与 UI 解释成本

## 4. 部署拓扑

## 4.1 总体拓扑

```text
                       Tailscale tailnet

   +--------------------------------------------------------------+
   |                                                              |
   |  Control-plane host                                          |
   |  - tailscaled                                                |
   |  - single-node k3s                                           |
   |  - Cylism Manager Pod                                        |
   |    - Web UI + API                                            |
   |    - SQLite PVC                                              |
   |    - hostPath: tailscaled.sock                               |
   |                                                              |
   |  Worker host A                                               |
   |  - tailscaled                                                |
   |  - k3s agent                                                 |
   |                                                              |
   |  Worker host B                                               |
   |  - tailscaled                                                |
   |  - k3s agent                                                 |
   |                                                              |
   +--------------------------------------------------------------+
```

## 4.2 平台部署前提

控制面宿主机必须满足：

- 已加入目标 tailnet
- 已安装并运行单节点 K3s
- 能通过 hostPath 向平台暴露 `tailscaled` socket
- 平台 Pod 固定调度在该控制面节点
- 提供持久化存储给 SQLite（当前清单使用控制面宿主机的 `hostPath`；生产环境应自行保障该目录的备份与权限）

## 4.3 平台运行前提

平台需要：

- 访问 Kubernetes API
- 访问宿主机 `tailscaled` socket
- 从平台 Pod 主动 SSH 到被管服务器
- 以单副本运行

## 4.4 建议的 Kubernetes 部署约束

- `replicas: 1`
- `nodeSelector` 或 `nodeAffinity` 固定到控制面节点
- `hostPath` 挂载 `tailscaled.sock`
- SQLite 使用 `/data/cylism-manager` hostPath
- Service 使用 `ClusterIP`
- 通过 Ingress 对外暴露 UI

## 5. 系统架构

## 5.1 组件划分

### 1. Web UI

负责：

- 页面展示
- 表单与预检交互
- 变更确认
- 审计上下文提示

### 2. Platform API

负责：

- 认证与授权
- 元数据 CRUD
- 编排 SSH 任务
- 聚合 Kubernetes API 数据
- 聚合 Tailscale LocalAPI 数据
- 记录审计日志

### 3. Metadata Store

负责保存：

- 服务器记录
- SSH 凭据密文
- 系统设置
- 扩展配置
- 操作审计
- 主机事实缓存

### 4. Tailscale Adapter

通过本机 `tailscaled` socket 读取：

- 本机 tailnet 状态
- peer 列表
- peer 在线状态
- peer Tailscale IP
- peer 基础标识

### 5. SSH Orchestrator

负责：

- 服务器连通性验证
- 激活检测
- 主机信息采集
- 节点加入脚本执行
- 节点清理脚本执行

### 6. Kubernetes Adapter

负责：

- 读取 Nodes、Namespaces、Workloads、Services、Configs、Routes
- 创建、更新、删除相应资源
- 检测扩展 CRD 是否存在

## 5.2 Agent 是可选运行时能力，不是主机纳管前提

平台的主机纳管和 K3s 编排仍采用 SSH + Kubernetes API；同时当前实现提供可选的 Agent/Runtime 能力，用于受控诊断、审批和运行时工具：

- 直接用 SSH 远程执行主机级操作
- 直接用 Kubernetes API 管理集群内资源
- 不要求所有被纳管服务器运行额外的常驻 Agent 进程
- Agent API 仅在对应 Runtime 或受控操作启用时使用

这样可以把方案真正收敛到“少组件、少故障点、可解释”。

## 6. 领域模型与数据归属

## 6.1 平台持久化对象

### Server

保存服务器台账与纳管信息。

建议字段：

```text
id
name
management_address
management_address_source   manual | tailscale
ssh_port
ssh_user
privilege_mode             root | sudo
ssh_auth_type              password | key
ssh_password_encrypted
ssh_key_encrypted
ssh_key_passphrase_encrypted
tailscale_device_id
tailscale_hostname
tailscale_ipv4
tailscale_online
activation_state
activation_message
labels_json
notes
created_at
updated_at
```

说明：

- `management_address` 是平台实际连接该主机使用的地址，默认优先 Tailscale IPv4
- `tailscale_*` 只是纳管辅助元数据，不是所有业务逻辑的核心
- 不在 `Server` 中持久化 `cluster_role`、`node_name` 作为权威事实

### ServerFactCache

缓存最近一次采集到的主机信息：

```text
server_id
os_name
arch
cpu_cores
memory_total_mb
disk_total_gb
kernel_version
uptime_seconds
last_collected_at
collect_error
```

### SystemSetting

保存系统级配置：

```text
key
value_encrypted
updated_at
```

主要内容：

- `tailscale_auth_key`
- `k3s_server_url`
- `k3s_join_token`
- `default_ssh_timeout`

### OperationLog

记录高风险操作与配置变更。

### ExtensionSetting

保存扩展安装状态与配置，如 cert-manager 集成配置。

## 6.2 实时对象

这些对象每次请求直接来自 K8s API：

- Cluster Node
- Namespace
- Deployment
- StatefulSet
- DaemonSet
- Pod
- Service
- ConfigMap
- Secret
- Ingress
- Certificate

## 6.3 服务器状态机

服务器状态建议统一为下面几种：

1. `discovered`
   仅从 Tailscale 导入，尚未补全 SSH 信息

2. `credential_pending`
   已创建记录，但凭据不完整

3. `connectivity_failed`
   凭据已填，但 SSH 登录失败

4. `ready`
   SSH 可登录，且基础激活检测通过

5. `joining_cluster`
   正在执行 worker 加入流程

6. `cluster_member`
   在 K8s API 中发现对应 Node

7. `drifted`
   数据库中存在服务器记录，但 K8s 中节点丢失，或映射异常

### 状态判定规则

- `cluster_member` 与 `drifted` 优先由 K8s 实时状态推导
- `ready` 及之前的状态由 SSH 连通性与预检结果推导

## 7. 数据源权威边界

| 领域 | 权威来源 |
|------|----------|
| 服务器台账 | SQLite |
| SSH 凭据 | SQLite（密文） |
| 导入结果 | SQLite |
| 主机事实缓存 | SQLite |
| 审计日志 | SQLite |
| Tailscale 设备清单 | 本机 tailscaled socket |
| K3s 节点状态 | Kubernetes API |
| 工作负载 / Service / Config / Route | Kubernetes API |
| 证书资源 | Kubernetes API + 扩展检测 |

这条边界必须保持清晰，否则后面很容易出现数据漂移和双写问题。

## 8. 关键业务流程

## 8.1 首次进入平台

### 前提

- K3s 已部署
- 平台已运行
- 控制面宿主机 Tailscale 已在线

### 页面行为

概览页展示：

- 平台状态
- Tailnet 连接状态
- 当前控制面节点状态
- 集群资源概要
- 未完成引导事项

若缺失 `tailscale_auth_key` 或 `k3s_join_token`，展示引导卡片，要求用户补全系统设置。

## 8.2 从 Tailscale 导入服务器

### 设计原则

- 导入操作必须先预览，再提交
- 默认允许“导入无冲突项”
- 若用户选择“严格模式”，则有任一冲突时整批失败

### 流程

1. 平台读取本机 tailnet peer 列表
2. 过滤掉本机控制面宿主机
3. 生成导入预览清单
4. 检测以下冲突：
   - `tailscale_device_id` 已存在
   - `management_address` 已存在
   - 用户指定名称重复
5. 页面展示：
   - 可导入
   - 已存在
   - 冲突待处理
6. 用户确认后提交导入

### 导入结果

新记录状态统一为：

- 有 SSH 信息时：`credential_pending`
- 无 SSH 信息时：`discovered`

## 8.3 激活服务器

“激活”不等于“能 ping 通”，而是指满足纳管基础条件。

### 激活检测项

- SSH 登录成功
- `sudo` 或 root 权限可用
- 操作系统受支持
- `systemd` 可用
- 磁盘空间足够
- 时间同步基本正常

激活通过后状态进入 `ready`。

## 8.4 加入 worker 节点

### 入口条件

- 服务器状态为 `ready`
- 服务器不属于当前集群
- 平台已有 `k3s_join_token`
- 目标服务器存在 Tailscale IPv4
  或平台已保存 `tailscale_auth_key` 可用于远程补装

### 流程

1. 运行集群加入预检
2. 若目标机未加入 tailnet，尝试安装并登录 Tailscale
3. 验证目标机可通过 Tailscale 访问控制面 `:6443`
4. 安装 `k3s agent`
5. 指定 `--node-ip=<target_tailscale_ipv4>`
6. 指向 `--server=https://<control_plane_tailscale_ipv4>:6443`
7. 等待 Node 出现在 K8s API
8. 校验 `Ready`
9. 记录审计日志

### 失败处理

- 若 agent 安装失败，保留服务器记录，不自动删除
- 若节点已注册但未 Ready，状态为 `drifted`
- 所有错误信息进入操作日志

## 8.5 查看服务器实时资源

V1 采用“按需拉取 + 短缓存”：

- 用户打开服务器详情时，通过 SSH 采集 CPU、内存、磁盘、负载
- 采集结果缓存 15 秒
- 不做持续采样时序库

这样能满足运维排障，又不会把平台做成监控系统。

## 8.6 管理配置 / 服务 / 路由

### 配置

- `ConfigMap` 与 `Secret` 分页签管理
- `Secret` 默认掩码显示
- 明文查看需要二次确认并写审计日志
- V1 支持 `Opaque` 与 `kubernetes.io/tls`

### 服务

V1 支持：

- `ClusterIP`
- `NodePort`
- `LoadBalancer`
- `Headless`

多端口 TCP/UDP Service 用于应用自身的流量暴露；平台不负责云厂商 LoadBalancer 的额外编排。

### 路由

通用路由优先使用标准 Kubernetes `Ingress` 模型：

- Host
- Path
- Backend Service + Port
- TLS Secret
- 常用注解

现有 Traefik `IngressRoute` 通过 `/api/routes` 提供独立的高级读取/操作入口，不等同于宿主机 NGINX 配置。

## 9. 页面与导航规划

## 9.1 顶层导航

建议保留三组一级导航：

1. `概览`
2. `基础设施`
3. `记录与系统`

## 9.2 页面清单

### 概览 `/`

展示：

- Tailnet 状态卡
- 控制面节点状态卡
- 集群资源总览
- 待激活服务器数
- 异常节点数
- 最近操作日志
- 引导事项卡片

### 服务器 `/servers`

只做“服务器台账与纳管”，不再和集群节点混在一个页签里。  
Tailscale 导入、SSH 纳管和激活入口都收敛在这里，系统级的 Auth Key 等配置放到“系统设置”。

列表字段建议：

- 名称
- 管理地址
- Tailscale IPv4
- Tailscale 在线状态
- SSH 用户
- 权限模式
- 激活状态
- 是否已在集群中
- 最近事实采集时间

详情页 / 侧栏展示：

- 基础信息
- SSH 配置
- 激活结果
- 主机事实信息
- 实时资源采集
- 审计记录

支持操作：

- 手动新增
- 导入
- 编辑
- 删除
- 测试 SSH
- 激活检测

### 集群节点 `/cluster`

专门展示 K8s Node 列表，不再塞进服务器页。

展示：

- 节点名
- 角色
- Ready 状态
- K8s 版本
- InternalIP
- CPU / 内存容量
- 对应服务器记录
- 异常状态

支持操作：

- 从“已激活且未入集群”的服务器中选择加入 worker
- Cordon / Drain / Remove
- 查看 Node Conditions

### 工作负载 `/resources?tab=workloads`

聚焦 Deployment / StatefulSet / DaemonSet。

支持视图切换：

- 按命名空间
- 按节点
- 按服务关联

V1 保持三类工作负载即可，不把 Job / CronJob 一次性塞进来。  
命名空间不再单独成页，而是作为工作负载等页面的统一筛选条件。

### 服务 `/resources?tab=services`

展示：

- Service 类型
- ClusterIP
- 端口
- Selector
- Endpoint 状态
- 所属命名空间

支持 CRUD 与详情查看。

### 配置 `/resources?tab=configs`

页签分为：

- ConfigMaps
- Secrets

Secret 相关交互必须强调风险。

### 路由 `/network?tab=routes`

展示当前 Ingress 规则：

- 域名
- 路径
- 后端 Service
- TLS
- 命名空间
- 生效状态

提供可视化创建与编辑，不暴露过多底层 YAML。

### 证书 `/network?tab=certificates`

该页受扩展能力控制：

- 若检测到 cert-manager CRD，则显示 `Certificate` 资源管理
- 若未安装，则展示“未启用证书扩展”说明与安装引导

这样可以满足用户的“证书管理类似服务”需求，同时避免把它硬绑定成核心前提。  
扩展状态不再独立成页，而是收敛到证书页和系统设置说明中。

### 审计 `/audit`

展示平台操作日志：

- 谁
- 在什么时间
- 对哪个对象
- 做了什么动作
- 是否成功

### 系统设置 `/settings/system`

集中管理：

- Tailscale auth key
- SSH 默认超时
- 控制面信息
- 平台基础配置
- 集群能力与扩展状态说明

### 数据管理 `/db-admin`

只保留为内部维护工具，不作为主路径暴露给普通产品导航。

## 10. 前后端结构建议

## 10.1 前端路由调整

当前前端主要路由：

```text
/
/servers
/cluster
/resources?tab=workloads|services|configs
/network?tab=routes|certificates
/audit
/settings/system
/db-admin
```

## 10.2 后端模块边界

建议拆成以下 handler / service：

- `network_handler` / `tailscale_service`
- `server_handler` / `server_activation_service`
- `cluster_handler` / `cluster_join_service`
- `namespace_handler`
- `workload_handler`
- `service_handler`
- `config_handler`
- `route_handler`
- `certificate_handler`
- `extension_handler`
- `audit_handler`
- `system_setting_handler`

集群加入、服务器激活、Secret 查看等高风险逻辑，不应全部堆在 handler 中。

## 11. 安全与审计要求

## 11.1 SSH 凭据

- 全部加密存储
- 接口不回传明文
- 更新采用写入式，不提供“回显现有密码”

## 11.2 Secret 管理

- 默认不明文展示
- 明文查看需二次确认
- 明文查看必须记审计

## 11.3 节点操作

以下动作必须写审计：

- 导入服务器
- 更新 SSH 凭据
- 激活服务器
- 加入 worker
- Drain / Remove Node
- 创建 / 删除 Route
- 查看 Secret 明文
- 启用 / 配置扩展

## 12. 与当前项目的重构方向

为了让现有项目朝这套方案演进，产品层面建议做这些调整：

1. 弱化旧的 `Sites / NGINX / acme.sh` 心智，不再作为主产品叙事。
2. 把“服务器”和“集群节点”拆成两个页面，避免台账与运行态混杂。
3. 把“路由”明确成 Ingress 管理，而不是泛化的宿主机转发。
4. 把“证书”改成扩展感知页面，而不是假设所有集群都天然支持。
5. 将 Tailscale 与导入逻辑收敛到“服务器”和“系统设置”，不再单独占一个基础设施页面。
6. 把 `db-admin` 从产品主导航降级为维护工具。

## 13. 分阶段落地建议

### 阶段 A：产品重定位与 IA 调整

- 完成导航精简与页面拆分
- 建立新产品文案与概览页
- 服务器/集群两层主结构定型

### 阶段 B：服务器纳管闭环

- Tailscale 扫描导入
- 服务器激活
- 主机事实采集

### 阶段 C：集群节点编排

- Worker 加入
- Node 映射
- Drain / Remove

### 阶段 D：K8s 资源主链路

- Namespace
- Workload
- Service
- Config
- Route

### 阶段 E：扩展能力

- 证书
- metrics-server

## 14. 结论

这套方案把 Tailscale 放在“网络与发现层”，把 K3s 资源管理放在“业务主层”，把 SQLite 放在“平台元数据层”，职责边界清晰，而且和你描述的真实部署流程一致：

- 用户先建 tailnet
- 再选一台机器建单节点 K3s
- 再部署平台
- 再导入和激活其他服务器
- 最后把已激活服务器加入为 worker，并统一管理集群资源

这样做既保留了 Tailscale 场景的核心价值，又避免把所有产品能力都绑死在 Tailscale 细节上，整体上是当前项目最可行的一条主线。
