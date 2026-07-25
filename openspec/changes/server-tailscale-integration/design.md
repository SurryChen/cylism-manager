# Cylism Manager — 服务器管理增强 & Tailscale 网络集成 设计文档

> 版本: v1.0 | 日期: 2026-07-25

## 1. 背景与目标

### 1.1 当前痛点

- 服务器表格信息太少（无 SSH 用户、认证方式）
-「加入集群」操作为 mock 空壳，未实现真实流程
- 跨公网/内网的 k3s 节点间网络不通（NAT 后内网机器无法被 control-plane 访问）
- `Server.Status` / `Server.LastSeen` 字段从未有效使用，`Dashboard.online_servers` 无可靠数据源

### 1.2 目标

1. 服务器管理信息完善（展示 SSH 凭据 + SSH 连通性检测）
2. 加入集群完整流程 + 实时进度日志（WebSocket）
3. 集成 Tailscale 解决跨网络节点通信，由本项目管理 Tailscale 全生命周期
4. 清理废弃字段

---

## 2. 数据库变更

### 2.1 Server 表变更

**删除字段：**

```sql
ALTER TABLE servers DROP COLUMN status;      -- 原 online/offline，无实际来源
ALTER TABLE servers DROP COLUMN last_seen;    -- 从未被赋值
```

**新增字段：**

```sql
ALTER TABLE servers ADD COLUMN tailscale_ip    VARCHAR(64) DEFAULT '';
ALTER TABLE servers ADD COLUMN tailscale_online TINYINT(1) DEFAULT 0;
```

**最终模型：**

```go
type Server struct {
    ID               uint           `gorm:"primaryKey" json:"id"`
    Name             string         `gorm:"size:128;not null" json:"name"`
    Host             string         `gorm:"size:256;uniqueIndex;not null" json:"host"`
    SSHHost          string         `gorm:"size:256" json:"ssh_host"`
    SSHPort          int            `gorm:"default:22" json:"ssh_port"`
    SSHUser          string         `gorm:"size:128" json:"ssh_user"`
    SSHAuthType      string         `gorm:"size:32" json:"ssh_auth_type"`   // password / key
    SSHPassword      string         `gorm:"type:text" json:"-"`
    SSHKey           string         `gorm:"type:text" json:"-"`
    SSHKeyPassphrase string         `gorm:"type:text" json:"-"`
    ClusterRole      string         `gorm:"size:32" json:"cluster_role"`    // "" | control-plane | worker
    K8sNodeName      string         `gorm:"size:256" json:"k8s_node_name"`
    TailscaleIP      string         `gorm:"size:64" json:"tailscale_ip"`
    TailscaleOnline  bool           `gorm:"default:false" json:"tailscale_online"`
    CreatedAt        time.Time      `json:"created_at"`
    UpdatedAt        time.Time      `json:"updated_at"`
    DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}
```

### 2.2 新增 system_configs 表

```sql
CREATE TABLE system_configs (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    key   VARCHAR(128) NOT NULL UNIQUE,
    value TEXT NOT NULL          -- 加密存储
);
```

```go
type SystemConfig struct {
    ID    uint   `gorm:"primaryKey" json:"-"`
    Key   string `gorm:"size:128;uniqueIndex" json:"key"`
    Value string `gorm:"type:text" json:"-"`  // 加密，不通过 API 返回明文
}
```

用途：存储 `tailscale_auth_key`、`k3s_join_token` 等系统级配置。

### 2.3 DashboardStats 变更

```go
type DashboardStats struct {
    TotalServers  int64 `json:"total_servers"`
    TotalSites    int64 `json:"total_sites"`
    ExpiringCerts int64 `json:"expiring_certs"`
}
// 删除 OnlineServers 字段
```

---

## 3. Tailscale 集成方案

### 3.1 方案选型

| 方案 | 说明 | 选择 |
|------|------|------|
| A. 仅脚本 | 用户手动装 Tailscale | ❌ |
| B. Tailscale mesh + k3s `--node-ip` | 本项目编排 Tailscale 全生命周期 + k3s 绑定 TS IP | ✅ |
| C. Tailscale K8s Operator | CRD 管理，杀鸡用牛刀 | ❌ |

### 3.2 网络拓扑

```
┌──────────────────────────────┐        ┌──────────────────────────────┐
│  公网机器 (control-plane)      │        │  内网机器 (worker)             │
│  Tailscale IP: 100.82.33.41  │◄──────►│  Tailscale IP: 100.92.15.72  │
│                              │WireGuard│                              │
│  k3s server:                 │  mesh  │  k3s agent:                  │
│  --node-ip=100.82.33.41      │        │  --node-ip=100.92.15.72      │
│  --advertise-address=        │        │  --server=https://            │
│    100.82.33.41              │        │    100.82.33.41:6443         │
└──────────────────────────────┘        └──────────────────────────────┘
```

k3s 所有内部通信（apiserver、flannel、kubelet）自动走 Tailscale WireGuard 隧道，NAT 穿透零配置。

---

## 4. Tailscale 全生命周期管理

本项目作为 Tailscale 的"编排者"，覆盖从初始化到日常运维。

### 4.1 阶段一：Tailscale 初始化（一次性）

**触发时机**：项目首次部署后，Dashboard 检测到 Tailscale 未初始化。

**前端交互（Dashboard Banner）**：

```
┌──────────────────────────────────────────────────────────────┐
│ ⚠ Tailscale 网络未初始化，集群节点间无法互通                    │
│                                                              │
│ 步骤 1: 前往 Tailscale 控制台获取 Auth Key                    │
│    → https://login.tailscale.com/admin/settings/keys          │
│                                                              │
│ 步骤 2: 填入下方并初始化                                       │
│    [Auth Key: tskey-auth-xxxxxxxxxxxx____________] [初始化]   │
│                                                              │
│ 或在本机手动执行:                                              │
│  curl -fsSL https://tailscale.com/install.sh | sh &&          │
│  tailscale up --auth-key=<your-key>                           │
└──────────────────────────────────────────────────────────────┘
```

**后端 POST /api/tailscale/init 流程：**

```
① which tailscale → 检测是否已安装
② (未安装) curl -fsSL https://tailscale.com/install.sh | sh
③ tailscale up --auth-key=$AUTH_KEY --hostname=control-plane
④ tailscale ip -4 → TS_IP
⑤ 加密存储 AUTH_KEY → system_configs 表
⑥ 读取 k3s token: cat /var/lib/rancher/k3s/server/node-token → 存储
⑦ 更新 DB: server.tailscale_ip=$TS_IP, server.tailscale_online=true
⑧ 返回 TS_IP + 状态
```

### 4.2 阶段二：加入集群（每个新节点）

**用户流程对照：**

```
用户在前端点击「加入集群」
  │
  ├── 步骤 1: 前置检测弹窗（自动执行）
  │     ├─ ① SSH 连接测试
  │     ├─ ② root 权限检测 (id -u)
  │     ├─ ③ swap 检测 (swapon --show)
  │     ├─ ④ OS 兼容性检测 (arch, systemd)
  │     └─ ⑤ 磁盘空间检测 (≥ 2GB)
  │
  ├── 步骤 2: 用户确认 → WebSocket 实时进度
  │     ├─ ⑥ 检测/安装 Tailscale
  │     ├─ ⑦ tailscale up --auth-key=$TS_AUTH_KEY
  │     ├─ ⑧ 获取 Tailscale IP
  │     ├─ ⑨ 通过 Tailscale IP 安装 k3s-agent
  │     ├─ ⑩ 等待 k3s-agent 服务启动
  │     ├─ ⑪ 轮询节点 Ready
  │     └─ ⑫ 写 DB
  │
  └── ✅ 完成，自动刷新节点列表
```

**关键点：**

- Auth Key 从阶段一存储的 `system_configs` 中读取，用户无需再次填写
- k3s join token 从阶段一读取的 token 中获取，用户无需手动查找
- 新节点只需能够 SSH 连接即可，其余全自动
- k3s agent 通过 Tailscale IP 连接 control-plane，不依赖公网 IP 可达

---

## 5. 加入集群 — 前置检测

### 5.1 检测项

| # | 检测项 | 方法 | 通过条件 |
|---|--------|------|---------|
| 1 | SSH 连接 | `ssh -o ConnectTimeout=5 host "echo ok"` | 5s 内返回 |
| 2 | Root 权限 | `id -u` | 输出为 `0` |
| 3 | Swap 状态 | `swapon --show` | 输出为空 |
| 4 | 操作系统 | `uname -m && cat /etc/os-release \| head -3` | x86_64/aarch64 + systemd |
| 5 | 磁盘空间 | `df -BG / \| tail -1 \| awk '{print $4}'` | ≥ 2GB |

### 5.2 接口

```
POST /api/servers/:id/precheck

Response:
{
  "checks": [
    { "name": "ssh_connect",   "label": "SSH 连接",   "pass": true,  "detail": "连接成功 (32ms)" },
    { "name": "root_privilege","label": "Root 权限",   "pass": true,  "detail": "uid=0" },
    { "name": "swap_disabled", "label": "Swap 状态",   "pass": false, "detail": "/dev/sda2 已开启 2GB swap" },
    { "name": "os_compatible", "label": "操作系统",    "pass": true,  "detail": "Ubuntu 22.04 x86_64" },
    { "name": "disk_space",    "label": "磁盘空间",    "pass": true,  "detail": "可用 45GB" }
  ],
  "all_pass": false
}
```

---

## 6. 加入集群 — WebSocket 进度日志

### 6.1 接口

```
ws://host/api/nodes/:id/join-progress

推送消息格式:
{
  "step": "check_tailscale",
  "label": "检测 Tailscale",
  "status": "running|success|failed",
  "detail": "Tailscale 已安装 (v1.80.3)",
  "index": 6,                    // 第几个步骤
  "total": 12,                   // 总步骤数
  "ts": "2026-07-25T10:30:01Z"
}

最后一条:
{
  "step": "complete",
  "label": "加入完成",
  "status": "success",
  "detail": "节点 iz7xvf... 已加入集群",
  "index": 12,
  "total": 12,
  "ts": "..."
}
```

### 6.2 前端弹窗 UI

```
┌──────────────────────────────────────────┐
│  加入集群 - 我的服务器 (192.168.1.100)     │  ← 标题栏
│                                          │
│  ┌─ 前置检测 ─────────────────────────┐  │
│  │ ✅ SSH 连接成功 (32ms)              │  │
│  │ ✅ Root 权限验证通过                │  │
│  │ ✅ Swap 已关闭                      │  │
│  │ ✅ Ubuntu 22.04 x86_64             │  │
│  │ ✅ 磁盘可用 45GB                    │  │
│  └────────────────────────────────────┘  │
│                                          │
│  ┌─ 进度日志 ─────────────────────────┐  │
│  │ ✅ 检测 Tailscale (已安装 v1.80.3) │  │
│  │ ⏳ 注册 Tailscale 网络...           │  │  ← 当前步骤闪烁
│  │ ◌ 获取 Tailscale IP               │  │
│  │ ◌ 安装 k3s-agent                  │  │
│  │ ◌ 等待服务启动                     │  │
│  │   (6/12 已完成)                    │  │
│  └────────────────────────────────────┘  │
│                                          │
│              [取消加入 (仅失败时可用)]      │
└──────────────────────────────────────────┘
```

全部通过后 3 秒自动关闭 + 刷新列表。

---

## 7. 服务器表格展示

### 7.1 变更前后对比

**变更前（当前）：**

| 名称 | 主机 | SSH 端口 | 集群角色 | 节点名 | 操作 |

**变更后：**

| 名称 | 主机 | SSH 用户 | 认证方式 | SSH 连通 | TS IP | TS 状态 | 集群角色 | 节点名 | 操作 |

### 7.2 列说明

| 列 | 来源 | 展示 |
|----|------|------|
| 名称 | `name` | 文字 |
| 主机 | `host` | 文字 |
| SSH 用户 | `ssh_user` | `root` |
| 认证方式 | `ssh_auth_type` | `密码` / `密钥` |
| SSH 连通 | `POST /api/servers/:id/probe` | 点击 🔍 → 弹窗：✅ `在线 (32ms)` / ✗ `不可达: connection refused` |
| TS IP | `tailscale_ip` | `100.82.33.41` 或 `-` |
| TS 状态 | `tailscale_online` | 🌐 在线 / `-` |
| 集群角色 | `cluster_role` | 徽章 |
| 节点名 | `k8s_node_name` | `iz7x...` 或 `-` |
| 操作 | - | 🔍 连通检测 · 加入集群 · 驱逐 · 移出 · 删除 |

### 7.3 SSH 连通性检测

```
POST /api/servers/:id/probe

后端：使用存储的 SSH 凭据 + 加密密钥解密 → 尝试 SSH 连接（5s 超时）

Response:
{ "reachable": true,  "latency_ms": 32 }
或
{ "reachable": false, "error": "connection refused" }
```

---

## 8. 接口清单

### 8.1 新增接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/tailscale/init` | 本机 Tailscale 初始化 |
| `GET` | `/api/tailscale/status` | Tailscale 状态 + `tailscale status` 输出 |
| `POST` | `/api/servers/:id/probe` | SSH 连通性快检 |
| `POST` | `/api/servers/:id/precheck` | 加入集群前置检测 |
| `WS` | `/api/nodes/:id/join-progress` | 加入集群实时进度 |

### 8.2 变更接口

| 方法 | 路径 | 变更 |
|------|------|------|
| `GET` | `/api/servers` | 响应增加 `tailscale_ip`/`tailscale_online`/`ssh_user`/`ssh_auth_type`；删除 `status`/`last_seen`/`sites` |
| `POST` | `/api/servers` | 删除 `Status: "offline"` 赋值 |
| `GET` | `/api/dashboard` | `stats` 中删除 `online_servers` |
| `POST` | `/api/nodes/:id/add` | 从 mock 改为完整实现 |

### 8.3 废弃接口

| 方法 | 路径 | 说明 |
|------|------|------|
| (无) | - | - |

---

## 9. 删除项清单

| 位置 | 内容 |
|------|------|
| `internal/model/models.go` — Server | `Status`、`LastSeen`、`Sites` 字段 |
| `internal/store/store.go` — DashboardStats | `OnlineServers` 字段 |
| `internal/store/store.go` — GetDashboardStats | `Where("status = ?", "online")` 查询 |
| `internal/store/store_test.go` | 所有对 `Server.Status` 的赋值和断言 |
| `internal/api/server_handler.go` | `Status: "offline"` 赋值 |
| `web/src/views/Dashboard.vue` | `在线` 指标卡片 |
| `web/src/views/Servers.vue` | 无（未曾使用 status/sites） |

---

## 10. 前端页面变更

### 10.1 Servers.vue

| 变更 | 说明 |
|------|------|
| 表格 +5 列 | SSH 用户、认证方式、SSH 连通性、TS IP、TS 状态 |
| SSH 连通性弹窗 | 行内 🔍 按钮触发 |
| 加入集群流程 | 删除原有简单确认弹窗 → 改为：前置检测弹窗 → WebSocket 进度弹窗 |
| 进度弹窗组件 | 可复用的 `ProgressModal.vue` |

### 10.2 Dashboard.vue

| 变更 | 说明 |
|------|------|
| 删除 `在线` 指标卡 | 只剩 3 张：服务器 / 站点 / 即将到期 |
| 新增 Tailscale Banner | 未初始化时提示配置 |

### 10.3 新增 `TailscaleConfig.vue`（系统设置页）

| 元素 | 说明 |
|------|------|
| 状态卡片 | Tailscale 已连接 / 未初始化 |
| Auth Key 配置 | 输入框 + 保存（加密存储） |
| 一键命令 | 复制即可执行：`tailscale up --auth-key=xxx` |
| 节点列表 | IP + 状态 + 最后在线 |

---

## 11. 部署脚本变更

### 11.1 新增 `scripts/install-tailscale.sh`

```bash
#!/bin/bash
set -e
AUTH_KEY="${1:?Usage: $0 <tskey-auth-xxx>}"
HOSTNAME="${2:-$(hostname)}"

curl -fsSL https://tailscale.com/install.sh | sh
tailscale up --auth-key="$AUTH_KEY" --hostname="$HOSTNAME" --accept-routes
echo "Tailscale IP: $(tailscale ip -4)"
```

### 11.2 k3s platform-deployment.yaml 变更

```yaml
env:
  - name: DATABASE_PATH
    value: /data/cylism.db
  - name: ENCRYPTION_KEY       # 新增：用于 system_configs 值加密
    valueFrom:
      secretKeyRef:
        name: cylism-secret
        key: encryption-key
```

---

## 12. 不在此次范围

- Tailscale ACL 管理界面
- 多 control-plane HA
- etcd 外部存储
- k3s 版本升级
- 节点资源监控告警
- 自动化 swap 关闭（需重启，风险高，交给用户）
