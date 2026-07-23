## 实现纪律（TDD）

> 以下所有实现任务均遵循 Test-Driven Development 流程：先编写测试用例（基于对应 spec 中的 WHEN/THEN 场景），确认测试失败，再编写实现代码，最后重构优化。每个任务完成时，其对应的测试必须全部通过。

## 1. 项目脚手架

- [x] 1.1 初始化 Go module 和项目目录结构（cmd/platform, cmd/agent, internal/, api/proto/, web/）
- [x] 1.2 配置 Viper 加载默认 config.yaml
- [x] 1.3 在 web/ 中初始化 Vue 3 + Vite 项目，搭建基础布局
- [x] 1.4 编写 Makefile，含 platform 和 agent 的构建目标

## 2. 数据模型与存储

- [x] 2.1 定义 GORM 模型：Server, Site, Cert, AuditLog
- [x] 2.2 实现 SQLite 存储层，含自动迁移和所有模型的 CRUD 方法
- [x] 2.3 实现 AES-256-GCM 加密/解密，用于 SSH 凭据字段

## 3. gRPC Agent 协议

- [x] 3.1 定义 agent.proto，包含所有 RPC（Ping, Nginx*, Acme*, File*, SystemInfo）
- [x] 3.2 从 proto 生成 Go gRPC 代码，创建 Agent server 骨架实现
- [x] 3.3 实现 Agent Ping 心跳处理器
- [x] 3.4 实现 Agent Nginx* 处理器（Test, Reload, GetConfig）
- [x] 3.5 实现 Agent Acme* 处理器（Issue, Renew, Revoke, Detect）
- [x] 3.6 实现 Agent File* 处理器（Read, Write, Stat）
- [x] 3.7 实现 Agent SystemInfo 处理器（操作系统/架构检测）

## 4. SSH 部署器

- [x] 4.1 使用 x/crypto/ssh 实现 SSH 客户端封装（密码和密钥认证）
- [x] 4.2 实现通过 SSH 探测远程操作系统和架构
- [x] 4.3 实现通过 SSH 上传 Agent 二进制并启动（systemd service + nohup 回退）
- [x] 4.4 实现 gRPC 客户端连接池，用于 Agent 通信

## 5. 服务器管理 API

- [x] 5.1 实现 POST /api/servers（注册服务器，含 SSH 凭据）
- [x] 5.2 实现 GET /api/servers 和 GET /api/servers/:id
- [x] 5.3 实现 PUT /api/servers/:id 和 DELETE /api/servers/:id
- [x] 5.4 实现 POST /api/servers/:id/deploy（触发 Agent 部署）
- [x] 5.5 实现后台 Agent 心跳监控，自动更新状态

## 6. 站点管理 API

- [x] 6.1 实现 POST /api/sites 和 GET /api/sites
- [x] 6.2 实现 GET /api/sites/:id, PUT /api/sites/:id, DELETE /api/sites/:id
- [x] 6.3 实现同一服务器内域名唯一性校验

## 7. NGINX 配置同步

- [x] 7.1 实现纯 HTTP server 块的 Go 模板
- [x] 7.2 实现 HTTPS server 块的 Go 模板（含 SSL 指令 + 重定向）
- [x] 7.3 实现 nginx -T 输出解析器，提取 server{} 块
- [x] 7.4 实现 POST /api/sites/:id/nginx/generate（渲染 + 通过 Agent 部署）
- [x] 7.5 实现 POST /api/sites/:id/nginx/reload
- [x] 7.6 实现 POST /api/nginx/import（解析 + 创建非受管站点）
- [x] 7.7 实现站点接管（managed=false → managed=true）
- [x] 7.8 实现删除受管站点时清理配置文件

## 8. 证书管理 API

- [x] 8.1 实现 acme.sh CLI 封装（签发、续期、吊销，支持验证方式选择）
- [x] 8.2 实现 POST /api/sites/:id/issue
- [x] 8.3 实现 POST /api/sites/:id/renew
- [x] 8.4 实现 POST /api/sites/:id/revoke
- [x] 8.5 实现后台证书到期扫描（30 天窗口，自动续期）

## 9. 仪表盘与审计

- [x] 9.1 实现 GET /api/dashboard（服务器/站点/证书统计 + 即将到期证书）
- [x] 9.2 实现审计日志中间件，记录所有变更类 API 调用
- [x] 9.3 实现 GET /api/audit-logs，支持分页和筛选

## 10. Web 前端

- [x] 10.1 构建仪表盘页面（概览卡片 + 到期证书列表）
- [x] 10.2 构建服务器管理页面（列表、添加表单含 SSH 认证、部署按钮、状态显示）
- [x] 10.3 构建站点管理页面（列表、创建/编辑表单、删除）
- [x] 10.4 构建站点详情页面（NGINX 配置生成、证书签发/续期/吊销操作）
- [x] 10.5 构建 NGINX 导入页面（触发导入、查看结果、接管操作）
- [x] 10.6 构建审计日志页面，支持筛选

## 11. 集成与收尾

- [x] 11.1 将 Vue SPA 构建产物嵌入 Go platform 二进制（embed.FS）
- [x] 11.2 端到端测试：注册服务器 → 部署 Agent → 创建站点 → 签发证书 → 生成配置
- [x] 11.3 全服务优雅关闭和错误处理
