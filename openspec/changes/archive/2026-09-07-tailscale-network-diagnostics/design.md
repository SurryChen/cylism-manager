# Tailscale Network Diagnostics Design

## Goals

1. 用 K3s 的实际配置识别官方内建 Tailscale，而非由 `100.64.0.0/10` 地址段推断。
2. 将“主机接入 Tailscale”“K3s 使用内建 Tailscale”“节点对实际路径”分开呈现。
3. 只使用固定、限时、只读 SSH 命令；不泄露凭据，也不提供通用远程执行能力。
4. 让服务器页能够定位跨节点镜像下载等流量问题的网络根因。

## Data Collection

平台从每个已登记服务器采集一次本机快照。SSH 命令由后端固定构造，浏览器不传入命令、主机地址、目标 IP 或文件路径。

### K3s mode

1. 读取 `k3s` 与 `k3s-agent` 的 active 状态。
2. 仅对 active 单元检测 `ExecStart`、`Environment` 是否包含 `--vpn-auth`、`--vpn-auth-file`、`K3S_VPN_AUTH` 或 `K3S_VPN_AUTH_FILE`。
3. 检查 K3s 标准配置目录中的 `vpn-auth` 与 `vpn-auth-file` 键是否存在。
4. 只返回布尔结论和 active unit 名称；任何匹配内容、文件内容和密钥均不返回。

Classification:

- `k3s_embedded_tailscale`: 活动 K3s 单元或 K3s 配置存在内建 VPN 标识。
- `external_tailscale`: Tailscale 可用，但没有 K3s 内建 VPN 标识。
- `standard_network`: 未发现 Tailscale。
- `unknown`: SSH、K3s 或 Tailscale 状态无法在固定期限内取得。

### Tailscale local state

仅在 `tailscale` 可执行时读取：

- `tailscale status --json`：本机 online、Tailnet IP、节点名称；不返回完整 peer 元数据。
- `tailscale netcheck --format=json`：UDP 可用性、IPv4 可用性、NAT 映射是否随目标变化、最近 DERP 区域。

解析失败时保留明确错误类别，不返回原始命令输出。

### Pair connectivity

平台从完成本机快照的节点提取 Tailnet IP，形成有向节点对。对每个源节点仅能探测同一已登记集合中的目标 Tailnet IP，使用固定、有界的 `tailscale ping`。

结果解析：

- `direct`: 成功输出 `via <endpoint>`，表示目前实际使用 UDP 直连。
- `derp`: 成功输出 `via DERP(<region>)`。
- `unreachable`: 固定期限内无成功响应或明确不可达。
- `unknown`: 源/目标不具备探测条件或输出无法解析。

向浏览器只返回路径类型、DERP 区域（若有）、延迟和错误类别；不返回公网 endpoint IP/端口。

## API

`GET /api/servers/network-diagnostics`

返回一次完整、只读的诊断快照：

```json
{
  "nodes": [{
    "server_id": 1,
    "name": "control-plane",
    "k8s_unit": "k3s",
    "network_mode": "k3s_embedded_tailscale",
    "tailscale": {
      "installed": true,
      "online": true,
      "tailnet_ip": "100.81.23.123",
      "udp": true,
      "mapping_varies_by_dest_ip": true,
      "nearest_derp": "Tokyo"
    }
  }],
  "links": [{
    "source_server_id": 1,
    "target_server_id": 2,
    "path": "direct",
    "latency_ms": 125
  }]
}
```

请求失败仅影响对应节点或链路，其他可完成的结果必须照常返回。

## UI

在 `/servers` 的现有分段导航中增加“网络诊断”。进入后显式点击刷新才执行探测，避免每次进入服务器列表触发跨节点请求。

- 顶部展示集群网络模式摘要：内建 Tailscale、外部 Tailscale、普通网络及未知节点数量。
- 节点表展示网络模式、K3s 活动单元、Tailscale 在线状态与 NAT 能力。
- 节点互联表展示有向链路的来源、目标、路径、延迟和异常状态。
- 使用现有 `SectionTabsHeader`、`data-table`、`badge` 和 icon button 样式；不添加说明性小字、营销文案或卡片嵌套。

“网络诊断”只反映当前检测结果，不提供“切换网络模式”操作。K3s 网络变更应继续由显式的基础设施配置流程执行。

## Security and Limits

- 所有 SSH 子命令及参数由后端静态定义，禁止前端指定命令或目标。
- 目标只可来自本次成功采集到的已登记服务器 Tailnet IP。
- 每个本机探测、每条 peer ping 都有短超时；并发数受限，节点或链路失败不会中断整批响应。
- 永不返回 `joinKey`、Auth Key、完整 systemd `ExecStart` / `Environment`、配置文件内容、Tailscale peer JSON 或公网端点。
- 诊断行为写入审计日志，但审计日志只记录服务器 ID、诊断结果和路径类型。

## Testing

- K3s 内建、外部、普通和未知网络模式解析。
- 仅活动 K3s 单元影响内建模式判断。
- Direct、DERP、超时与不可解析 ping 输出分类。
- API 拒绝或忽略任意外部目标；返回内容不包含凭据、join key 或公网 endpoint。
- 前端展示混合节点、局部失败、DERP 与 direct 结果，并验证刷新交互。
