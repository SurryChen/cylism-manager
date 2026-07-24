## 背景

当前站点管理（/sites）和 NGINX 导入（/import）是两个独立页面，且后端 NGINX 操作全部是 stub。站点 CRUD 可用，但配置生成、重载、导入全不可用。需要整合页面并补全后端实现。

## 目标 / 非目标

**目标：**
- 站点页统一管理受管站点 + 导入候选站点
- NGINX 配置生成：Agent 渲染模板 → 写入 → nginx -t 校验 → reload
- NGINX 重载：Agent 执行 nginx -s reload
- NGINX 导入：Agent 自动探测主机/Docker nginx → nginx -T → 解析 server 块返回
- Docker 支持：docker ps + docker exec 自动探测
- 移除独立的"导入"导航项

**非目标：**
- NGINX 配置可视化编辑器（一期用模板渲染）
- 自定义配置片段管理（upstream/location 已有字段，但不做在线编辑）

## 技术决策

### 1. 前端整合方案

```
站点页面布局：
┌──────────────────────────────────────────┐
│ [服务器: 全部 ▼]           [+ 添加站点]  │
│                                          │
│ [受管站点] [导入站点]                    │
│ ┌──────────────────────────────────────┐ │
│ │  表格（根据 tab 切换数据源）          │ │
│ └──────────────────────────────────────┘ │
└──────────────────────────────────────────┘
```

### 2. NGINX 模板渲染

配置模板位于 `internal/nginx/templates/`:
- `server-http.conf.tmpl`: HTTP only server block
- `server-https.conf.tmpl`: HTTPS server block + HTTP redirect

渲染用 Go `text/template`，从 Site 模型填充变量。

### 3. NGINX 导入流程

```
POST /api/nginx/import { server_id: 1 }
  → Agent.DetectNginx(ctx, req)  // gRPC
    → 1. 先试 nginx -T（主机安装）
    → 2. 若失败，docker ps | grep nginx
    → 3. docker exec <container> nginx -T
    → 返回：nginx 安装方式 + 解析出的 server 块列表
  → 后端解析 server_name、listen、root、proxy_pass、ssl_* 等指令
  → 返回候选站点列表（不持久化）
```

### 4. Docker 探测

Agent 新增 gRPC RPC: `DetectNginx`，返回：

```protobuf
message DetectNginxResponse {
  string install_type = 1;  // "host" | "docker"
  string container_name = 2;
  repeated ServerBlock server_blocks = 3;
}
```

### 5. 接管流程

```
POST /api/sites/takeover { domain, server_id, port, root_path, ssl_enabled, ... }
  → 创建 managed=true 的 Site 记录
  → 返回创建的站点
```

## 风险与权衡

- **[多容器 NGINX]** Docker host 上可能有多个 nginx 容器 → 返回第一个匹配的，用户可通过配置指定
- **[配置解析不完整]** nginx 配置语法极其灵活，正则解析可能漏掉部分指令 → 一期覆盖 server_name/listen/root/proxy_pass/ssl_*，其余标记为未识别
