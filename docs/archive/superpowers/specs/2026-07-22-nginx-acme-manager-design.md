# Cylism Manager — NGINX & acme.sh 管理平台 设计文档

> 日期: 2026-07-22 | 状态: Draft

## 1. 概述

Cylism Manager 是一款面向小团队的 NGINX + acme.sh 管理平台。部署在单台服务器上，通过 Web UI 管理 NGINX 站点配置和 SSL 证书生命周期，支持远程服务器通过 SSH 部署 Agent 并建立 gRPC 长连接管理。

## 2. 核心需求

| 维度 | 决策 |
|------|------|
| 部署场景 | 单服务器部署，Web UI 管理，支持多台远程服务器 |
| NGINX 配置 | 双向同步（元数据驱动生成 + 反向导入已有配置） |
| acme.sh | 平台内置 CLI 封装调用 |
| Agent 通信 | gRPC，长连接管理 |
| SSH | 支持密码/密钥认证，远程部署 Agent |
| 用户 | 单用户优先，架构预留多用户 |
| 后端 | Go |
| 前端 | Vue SPA |
| 存储 | SQLite |

### 非目标（一期不做）

- 多用户认证与权限
- 非 NGINX 的 Web Server 支持
- DNS 托管服务集成

## 3. 架构

```
┌─────────────────────────────────────────┐
│              Web UI (Vue SPA)            │
└─────────────────┬───────────────────────┘
                  │ REST API
┌─────────────────▼───────────────────────┐
│           Go 管理平台 (Platform)          │
│  ┌──────────┐ ┌────────┐ ┌───────────┐ │
│  │ Site API │ │Cert API│ │ Server API│ │
│  └────┬─────┘ └───┬────┘ └─────┬─────┘ │
│  ┌────▼───────────▼────────────▼─────┐  │
│  │          Service Layer            │  │
│  │  ┌──────┐ ┌──────┐ ┌──────────┐ │  │
│  │  │Nginx │ │Acme  │ │Deployer  │ │  │
│  │  │Svc   │ │Svc   │ │(SSH)     │ │  │
│  │  └──────┘ └──────┘ └──────────┘ │  │
│  │  ┌──────┐ ┌──────────────────┐  │  │
│  │  │Config│ │  Agent Manager   │  │  │
│  │  │Import│ │  (gRPC Client)   │  │  │
│  │  └──────┘ └──────────────────┘  │  │
│  └────────────────┬─────────────────┘  │
│  ┌────────────────▼─────────────────┐  │
│  │        SQLite (元数据)            │  │
│  └──────────────────────────────────┘  │
└─────────────────┬───────────────────────┘
                  │
     ┌────────────┼────────────┐
     ▼            ▼            ▼
┌─────────┐ ┌─────────┐ ┌─────────┐
│ Agent A │ │ Agent B │ │ Agent C │
│ (gRPC)  │ │ (gRPC)  │ │ (gRPC)  │
└─────────┘ └─────────┘ └─────────┘
  服务器1     服务器2     服务器3
```

### Agent 生命周期

```
用户添加服务器 (host + SSH 认证)
       │
       ▼
 Platform 通过 SSH 连接远程服务器
       │
       ▼
 上传 Agent 二进制 → 启动 Agent (gRPC Server)
       │
       ▼
 Platform 建立 gRPC 长连接 → 心跳保活
       │
       ▼
 通过 gRPC 下发操作指令 (nginx reload, acme.sh issue 等)
```

## 4. 数据模型

### 4.1 Server（服务器）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| name | TEXT | 服务器名称/别名 |
| host | TEXT | 主机地址 |
| port | INTEGER | Agent gRPC 端口，默认 9527 |
| ssh_host | TEXT | SSH 连接地址（可与 host 不同） |
| ssh_port | INTEGER | SSH 端口，默认 22 |
| ssh_user | TEXT | SSH 用户名 |
| ssh_auth_type | TEXT | password / key |
| ssh_password | TEXT | SSH 密码（加密存储，auth_type=password 时使用） |
| ssh_key | TEXT | SSH 私钥内容（加密存储，auth_type=key 时使用） |
| ssh_key_passphrase | TEXT | 私钥密码（可选） |
| status | TEXT | online / offline / deploying |
| last_seen | DATETIME | 最后心跳时间 |

### 4.2 Site（站点）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| server_id | INTEGER FK | 所属服务器 |
| domain | TEXT | 主域名（同一 server 下唯一） |
| port | INTEGER | 监听端口，默认 80/443 |
| ssl_enabled | BOOLEAN | 是否启用 HTTPS |
| root_path | TEXT | Web 根目录 |
| managed | BOOLEAN | 是否由平台管理 |
| nginx_conf_path | TEXT | NGINX 配置片段路径 |
| upstream | JSON | 上游代理配置 |
| locations | JSON | 额外 location 规则 |
| cert_id | INTEGER FK | 关联的证书 |

### 4.3 Cert（证书）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| site_id | INTEGER FK | |
| domains | JSON | SAN 域名列表 |
| provider | TEXT | 固定 "acme.sh" |
| account | TEXT | ACME 账户标识 |
| cert_path | TEXT | 证书文件路径 |
| key_path | TEXT | 私钥文件路径 |
| fullchain_path | TEXT | 完整链路径 |
| valid_from | DATETIME | 生效时间 |
| valid_to | DATETIME | 到期时间 |
| status | TEXT | issued / renewing / expired / revoked |
| challenge | TEXT | http / dns |
| last_renew | DATETIME | 上次续期时间 |

### 4.4 AuditLog（审计日志）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| action | TEXT | create / update / delete / issue / renew / reload / deploy |
| resource_type | TEXT | site / cert / server |
| resource_id | INTEGER | |
| detail | JSON | 变更详情 |
| created_at | DATETIME | |

## 5. SSH 远程管理

### 5.1 添加服务器流程

1. 用户在 UI 填写：服务器名、host、SSH 认证方式
2. Platform 尝试 SSH 连接，验证认证信息
3. 连接成功后：
   - 探测远程服务器 OS/架构
   - 上传匹配的 Agent 二进制
   - 通过 SSH 启动 Agent（systemd 或 nohup）
4. Platform 建立 gRPC 连接，开始心跳
5. Server 状态更新为 `online`

### 5.2 安全

- SSH 密码/私钥使用 AES-256 加密后存入 SQLite
- gRPC 使用 TLS 双向认证

## 6. NGINX 双向同步

### 6.1 元数据 → 配置（生成）

1. 根据 Site 元数据渲染 Go template
2. 通过 Agent gRPC 下发：写入 `conf.d/<domain>.conf`
3. 通过 Agent 执行 `nginx -t` 校验
4. 校验通过后 `nginx -s reload`

### 6.2 配置 → 元数据（导入）

1. 通过 Agent 执行 `nginx -T` 获取运行时配置
2. Platform 解析 `server {}` 块
3. 匹配已有元数据（按 domain），更新 `nginx_conf_path`
4. 未匹配的标记为 `managed=false`

### 6.3 冲突处理

- 元数据为权威来源，导入配置不覆盖已有元数据
- 用户可选择"接管"导入站点（设为 managed=true）

## 7. acme.sh 集成

### 7.1 签发

1. Platform 检测目标服务器是否安装 `acme.sh`（通过 Agent）
2. 选择验证方式后，Agent 执行 `acme.sh --issue ...`
3. 签发成功后路径写入 Cert 元数据
4. 自动更新关联 Site 的 NGINX 配置

### 7.2 续期

- 定时任务扫描即将过期（<30天）的证书
- Agent 执行 `acme.sh --renew -d <domain>`
- 续期成功 → `nginx -s reload`

### 7.3 吊销

- 用户手动触发，Agent 执行 `acme.sh --revoke -d <domain>`
- 清理 Cert 元数据，移除 NGINX SSL 配置

## 8. gRPC 服务定义（Proto）

```protobuf
service Agent {
  // 心跳
  rpc Ping(PingReq) returns (PingResp);
  // NGINX 操作
  rpc NginxTest(NginxTestReq) returns (NginxTestResp);
  rpc NginxReload(NginxReloadReq) returns (NginxReloadResp);
  rpc NginxGetConfig(NginxGetConfigReq) returns (NginxGetConfigResp);
  rpc WriteConfigFile(WriteConfigFileReq) returns (WriteConfigFileResp);
  // acme.sh 操作
  rpc AcmeIssue(AcmeIssueReq) returns (AcmeIssueResp);
  rpc AcmeRenew(AcmeRenewReq) returns (AcmeRenewResp);
  rpc AcmeRevoke(AcmeRevokeReq) returns (AcmeRevokeResp);
  rpc AcmeDetect(AcmeDetectReq) returns (AcmeDetectResp);
  // 文件操作
  rpc FileRead(FileReadReq) returns (FileReadResp);
  rpc FileWrite(FileWriteReq) returns (FileWriteResp);
  rpc FileStat(FileStatReq) returns (FileStatResp);
  // 系统信息
  rpc SystemInfo(SystemInfoReq) returns (SystemInfoResp);
}
```

## 9. API 设计

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/dashboard | 仪表盘概览 |
| GET | /api/servers | 服务器列表 |
| POST | /api/servers | 添加服务器（含 SSH 认证信息） |
| GET | /api/servers/:id | 服务器详情 |
| PUT | /api/servers/:id | 更新服务器 |
| DELETE | /api/servers/:id | 删除服务器 |
| POST | /api/servers/:id/deploy | 部署 Agent |
| GET | /api/sites | 站点列表 |
| POST | /api/sites | 创建站点 |
| GET | /api/sites/:id | 站点详情 |
| PUT | /api/sites/:id | 更新站点 |
| DELETE | /api/sites/:id | 删除站点 |
| POST | /api/sites/:id/issue | 签发证书 |
| POST | /api/sites/:id/renew | 续期证书 |
| POST | /api/sites/:id/revoke | 吊销证书 |
| POST | /api/sites/:id/nginx/generate | 生成 NGINX 配置 |
| POST | /api/sites/:id/nginx/reload | 重载 NGINX |
| POST | /api/nginx/import | 从 NGINX 导入配置（支持指定 server_id） |
| GET | /api/audit-logs | 审计日志 |

## 10. 项目结构

```
cylism-manager/
├── cmd/
│   ├── platform/          # 管理平台入口
│   └── agent/             # Agent 入口
├── api/
│   └── proto/
│       └── agent.proto    # gRPC 服务定义
├── internal/
│   ├── api/               # REST API handlers
│   ├── service/           # 业务逻辑
│   │   ├── site/
│   │   ├── cert/
│   │   ├── nginx/
│   │   ├── server/
│   │   └── deployer/      # SSH 部署逻辑
│   ├── model/             # 数据模型
│   ├── store/             # SQLite 数据访问
│   ├── nginx/             # NGINX 配置解析/生成
│   ├── acme/              # acme.sh CLI 封装
│   ├── agent/             # gRPC Agent 客户端
│   └── crypto/            # SSH 凭据加密/解密
├── web/                   # Vue SPA
├── config/                # 配置文件
└── docs/                  # 文档
```

## 11. 技术选型明细

| 组件 | 选型 | 说明 |
|------|------|------|
| HTTP 路由 | chi 或 gin | |
| ORM | GORM 或 sqlx | |
| 配置管理 | Viper | |
| gRPC | google.golang.org/grpc | Agent 通信 |
| SSH | golang.org/x/crypto/ssh | 远程连接与部署 |
| NGINX 解析 | 手写解析器 | server{} 块提取 |
| 前端 | Vue 3 + Vite | SPA |
| UI 组件 | 待定 | Element Plus 或 Naive UI |
| 凭据加密 | crypto/aes | SSH 密码/私钥加密存储 |
