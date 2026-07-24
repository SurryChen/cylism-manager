## Why

站点管理和 NGINX 导入当前是独立的导航页（`/sites` + `/import`），但本质上是同一件事：管理服务器的 Web 站点配置。NGINX 的生成、重载、导入全部是空壳 stub。站点页面有按钮但点了无效，导入页面有完整 UI 但后端无法返回数据。需要整合页面并补全后端实现。

## What Changes

- 移除"导入"导航项，站点页整合为唯一入口
- 站点页改为双 tab：受管站点 + 导入站点
- 补全 NGINX 配置生成（gRPC Agent 渲染模板 → 写入 → nginx -t → reload）
- 补全 NGINX 重载（gRPC Agent nginx -s reload）
- 补全 NGINX 导入（Agent 自动探测主机/Docker nginx → nginx -T → 解析返回）
- Agent 新增 Docker 探测：docker ps + docker exec nginx -T
- 接管导入站点时创建 managed Site 记录

## Capabilities

### 修改能力

- `ui-navigation`: 移除"导入"入口
- `nginx-site-management`: 整合站点 & NGINX 管理，双 tab + 服务器筛选
- `agent`: 新增 Docker 容器探测 + nginx -T 支持

## 影响范围

- 后端：`internal/api/site_handler.go` — 实现 GenerateNginx/ReloadNginx
- 后端：`internal/api/nginx_handler.go` — 实现 Import（Agent 探测 + 解析）
- 后端：`internal/agent/` — 新增 Docker 探测 RPC
- 后端：`internal/nginx/` — NGINX 配置模板渲染 + 解析
- 前端：`web/src/views/Sites.vue` — 改造为双 tab + 服务器筛选
- 前端：`web/src/views/NginxImport.vue` — 删除
- 前端：`web/src/App.vue` — 移除导入导航
- 前端：`web/src/router/index.js` — 移除 /import 路由
