## 为什么

小团队管理多台服务器上的 NGINX 站点和 SSL 证书目前依赖手工操作：手动编辑配置文件、手动运行 acme.sh 签发续期证书、手动 reload。这种方式容易出错（配置漂移、证书过期遗忘），且缺乏统一的元数据视角来追踪站点与证书的关联关系。需要一个平台来集中管理这些操作，降低运维门槛。

## 变更内容

- 新增 NGINX 站点管理：通过元数据驱动生成 NGINX 配置，同时支持从已有配置反向导入
- 新增 acme.sh 证书生命周期管理：签发、续期、吊销，支持 HTTP 和 DNS 验证方式
- 新增多服务器管理：通过 SSH 远程部署 Agent，建立 gRPC 长连接下发操作指令
- 新增 SSH 凭据管理：支持密码和密钥认证，凭据加密存储
- 新增仪表盘和审计日志：证书到期提醒、操作记录追踪
- 新增 Web UI (Vue SPA)：可视化管理所有站点、证书和服务器

## 能力

### 新增能力

- `server-management`: 服务器注册、SSH 认证配置、Agent 部署与状态监控
- `site-management`: NGINX 站点增删改查、元数据存储、NGINX 配置生成
- `cert-management`: acme.sh 证书签发/续期/吊销、证书到期监控
- `nginx-config-sync`: NGINX 配置双向同步（元数据生成 + 反向导入）
- `dashboard-audit`: 仪表盘概览、操作审计日志

### 修改的能力

<!-- 新项目，无已有 spec 需要修改 -->

## 影响范围

- 新增 Go 后端项目（platform + agent 两个二进制）
- 新增 Vue SPA 前端项目
- 新增 SQLite 数据库存储元数据
- 新增 gRPC proto 定义（Agent 通信协议）
- 依赖：本机需安装 acme.sh，被管服务器需有 NGINX 和 SSH 访问
