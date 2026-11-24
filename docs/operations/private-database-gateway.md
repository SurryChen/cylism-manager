# 私网数据库网关

入口：**云服务 → 数据库 → 已有实例 → 接入已有 MySQL**。选择“通过私网网关连接”，只需登记实例名称、内网地址和端口，即可进入网关配置；此时不需要业务数据库或账号。

登记实例和私网接入用于平台内的连接管理，不会导入业务数据，也不包含阿里云 RDS 的规格、备份或账号等云 API 管理。

```text
应用 Pod
  → cylism-db-gateway-<网关ID>.cylism-databases.svc.cluster.local:3306
  → EndpointSlice（网关 Tailscale IP / 专属端口）
  → 网关 ECS 上的 systemd TCP 转发服务
  → RDS 内网域名:3306
```

## 准备条件

- 选择能够访问 RDS 私网端点的服务器，通常为 RDS 同地域、同 VPC 的 ECS。平台实际检查 DNS、私网 IP 和 MySQL 握手，不通过云 API 推断 VPC 归属。
- 网关已经加入 Tailscale，并配置相应的 Tailnet 访问策略；应用节点必须具有到网关的路由。
- 网关使用 Linux、systemd、Python 3.7+、`flock`、`install`、`base64`、`tailscale`；平台的 SSH 用户需要 root 或 `sudo -n` 权限。部署时先检查 `/usr/bin/python3`，再检查 `/usr/bin/python3.15` 至 `/usr/bin/python3.7`，选择符合要求的解释器。系统默认 Python 过旧时，可通过系统官方软件源并行安装较新版本，无需替换系统默认解释器；没有可用版本时，任务会在安装网关前明确失败。
- RDS 需要允许网关在 VPC 内的实际来源地址；填写的是 **ECS 内网 IP**，不是它的 Tailscale IP。平台不会修改云安全组、RDS 白名单、账号或 Tailscale 路由。
- 集群节点能拉取诊断镜像。默认 `python:3.12-alpine`，可改为包含 Python 3 的可信私有镜像。

## 配置步骤

1. 在“已有实例”登记实例名称、RDS **内网域名**和端口；选择“通过私网网关连接”。已有数据库和业务账号可以后续再配置。
2. 进入“私网网关”，选择服务器、应用集群、该服务器实际的 Tailscale IPv4 和一个专用监听端口（如 13306）。
3. 填写允许来源。节点 SNAT 后通常是各节点 Tailscale IP `/32`；如果 CNI 保留 Pod 源地址，则需要精确配置对应 Pod CIDR，并保证返回路由可用。不要直接开放整个 Tailnet 或所有私网来掩盖路由问题。
4. 保存后后台检查网关身份、数据库域名只解析到 RFC1918 地址、目标 MySQL 握手，然后安装受管转发程序。
5. 创建无 selector 的 ClusterIP Service 和 EndpointSlice。每个 Ready 且可调度节点运行一个临时、非特权、无 ServiceAccount 令牌的诊断 Pod，连续连接 Service 三次并读取 MySQL 握手。
6. 全部通过后状态为“网络已验证”。应用需要使用时，再进入实例的“业务连接”，填写已存在的数据库名、业务用户名和密码。平台只保存连接信息，不会执行建库或创建账号；密码由平台加密保存，不会传给网关程序。
7. 配置业务连接后，再绑定到该集群的应用模板并发布应用。未配置业务连接时，平台拒绝应用绑定，但仍可独立部署和验证网关。

修改业务连接时需重新填写账号的现有密码。已绑定上线模板的业务连接需先解除绑定，修改后重新绑定，避免模板继续使用旧凭据。此操作不会修改 MySQL 内的账号密码。已有携带完整业务连接的实例及旧客户端仍兼容，集群托管实例仍要求完整的初始化数据库和账号。

已有直接连接的实例若已被应用绑定，需要先解除绑定再配置网关，以免现有模板仍指向旧地址。选择私网网关接入但尚未完成配置的实例不能绑定；网关失败、停用或属于其他集群时也不能绑定。服务器、集群和监听地址在网关创建后固定；允许来源和诊断镜像可修改并重新部署。

## TLS 与验证范围

转发程序不终止 TLS，MySQL 协议数据原样传递。需要 TLS 时，应用必须启用可信 CA 和主机名校验，校验名称应为 **原始 RDS 域名**，不能使用网关 Service 域名。平台弹窗单独展示此名称；通用应用绑定不会猜测或自动注入某个驱动特有的 TLS 参数。

“网络已验证”表示 **Pod → DNS/Service → Tailscale → 网关 → 私网 MySQL 握手** 通过，不代表数据库密码、权限、SQL 或 TLS 身份已验证。应用发布后仍需验证实际数据库连接。网关只支持 IPv4，目标只允许 RFC1918 私网地址，来源只允许显式的 Tailscale 或 RFC1918 IPv4/CIDR。

## 任务与恢复

- 配置及部署/测试/停用任务先持久化到平台 SQLite，再由 bootstrap 后台循环执行。任务包含阶段日志，可关闭页面后重新打开查看。每个网关只允许一个进行中的任务。
- 当前执行器按平台单实例部署设计；平台正常退出或进程崩溃后，下一次启动会重复检查并恢复未完成任务。操作可重复执行，远端部署和停用通过专属 `flock` 串行执行。
- 不设置整项任务的总截止时间；SSH、DNS、单次连接和单个诊断 Pod 有局部超时，以免失联连接永久卡住队列。失败后保留配置、日志和已创建的 Service，修正条件后显式重新部署或测试。
- 临时 Pod 在测试结束或取消时清理；进程硬崩溃遗留的诊断 Pod 在恢复测试前清理。不会清理其他工作负载。
- 转发服务使用 `cylism-db-gateway-<ID>.service`，由 systemd 开机启动和异常重启。`Type=notify` 确保监听成功后才报告部署启动成功。配置和脚本位于 `/etc/cylism/cylism-db-gateway-<ID>/`，仅包含地址和来源限制；该目录的 `python3` 链接固定部署时选中的解释器，服务启动和健康检查共用它。旧版网关没有该链接时，健康检查会回退到系统解释器检测。
- 后台约每分钟检查网关进程、Tailscale IP、本地监听端口和目标私网 DNS，不向数据库建立连接；“重新测试”重新执行各节点 Pod 验证。两种检查时间分别展示，后台检查不替代完整链路验证。
- 停用会停止转发并取消开机启动，再移除平台拥有的 EndpointSlice 和 Service。网关脚本和无密码配置保留，便于再次启用。外部数据库、数据库账号和业务数据均保留。删除实例记录前必须先停用并解除应用绑定。

单个网关是单点故障，本版本不包含双网关高可用、数据库迁移、RDS 云 API 管理或自动修改应用的 TLS 配置。

## 发布要求

更新平台镜像并同步部署清单中的 RBAC；本功能需要 `discovery.k8s.io/endpointslices` 的 `create`、`update`、`delete` 权限，此外复用现有的 Namespace、Service、Pod、Pod 日志和 Node 权限。Helm 和 `k8s/platform-deployment.yaml` 均已更新。仅重启旧 Deployment 而未更新权限时，后台任务会在创建 EndpointSlice 阶段失败。

本地验证：

```sh
go test ./internal/service/database/gateway ./internal/api/cloud ./internal/bootstrap ./internal/store ./internal/api
python3 -B -m unittest discover -s internal/service/database/gateway -p '*_test.py' -v
npm --prefix web test -- src/views/cloud/DatabaseGatewayDialog.test.js
npm --prefix web run build
```

本地测试不连接真实 RDS，也不在真实服务器安装服务。首次上线需要通过上述平台任务完成真实环境验证。
