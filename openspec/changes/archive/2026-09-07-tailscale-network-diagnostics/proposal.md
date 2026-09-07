## Why

服务器页当前只能展示登记信息、SSH 连通性和资源采样。平台无法分辨 K3s 是使用官方内建 Tailscale 集成、仅安装了外部 Tailscale，还是未使用 Tailscale；也无法说明节点对之间实际经由 UDP 直连、DERP 中继或不可达。

当集群跨云或跨网络部署时，这会掩盖节点间链路问题。例如镜像代理本机缓存读取正常，但跨节点经 Tailscale 的下载速度可能严重退化。管理员需要在平台内获得可验证、脱敏的网络路径证据。

## What Changes

- 在“服务器”页面新增“网络诊断”视图，按节点展示 K3s 网络模式与 Tailscale 本机状态。
- 通过固定、只读的 SSH 探测识别活动 K3s 服务是否配置官方 `--vpn-auth`、`--vpn-auth-file` 或等价配置文件字段；不会读取或返回 join key。
- 为已接入 Tailscale 的已登记服务器生成节点对连通性结果，分类为 UDP 直连、DERP 中继、不可达或未知。
- 展示每个节点的 Tailscale `netcheck` 能力摘要，使“节点具有 Tailscale”与“节点间连接质量正常”成为两个独立判断。
- 为 API、解析逻辑和页面状态添加测试；诊断仅接受受控的服务器记录，不提供任意主机、IP 或命令输入。

## Out of Scope

- 不自动修改 K3s 的 `vpn-auth`、`node-external-ip`、Flannel 或 Tailscale 配置。
- 不读取、持久化或向浏览器返回 Tailscale Auth Key、join key、VPN 配置文件内容或完整命令行。
- 不以单次诊断结果替代持续带宽监控；本次只提供路径、可达性、延迟与 NAT 能力证据。

## Capabilities

### New Capabilities

- `tailscale-network-diagnostics`: 检测 K3s 内建 Tailscale 配置、节点 Tailscale 状态、NAT 能力和节点对实际连接路径。

### Modified Capabilities

- `server-management`: 在服务器页提供网络诊断入口和结构化诊断结果。

## Impact

- 后端：服务器诊断 handler、新路由、SSH 探测及安全解析逻辑。
- 前端：`Servers.vue` 新增网络诊断视图与结果展示。
- API：新增只读网络诊断接口。
- 不涉及数据库迁移或集群配置写入。
