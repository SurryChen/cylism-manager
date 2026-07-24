## 实现任务

### 1. NGINX 模板渲染模块
- [x] `internal/nginx/template.go` — 实现 HTTP/HTTPS server 块模板渲染（text/template）
- [x] `internal/nginx/parser.go` — 实现 nginx -T 输出解析（提取 server_name/listen/root/proxy_pass/ssl_*）
- [x] 编写模板渲染和解析的单元测试

### 2. Agent Docker 探测 + DetectNginx RPC
- [x] `api/proto/agent/agent.proto` — 新增 DetectNginx RPC + 消息
- [x] `cmd/agent/main.go` — 实现 DetectNginx handler（主机 nginx -T + docker ps + docker exec）
- [x] `internal/agent/server.go` — 新增 DetectNginx 方法
- [x] 重新生成 proto stub

### 3. 后端 NGINX Handler 实现
- [x] `internal/api/nginx_handler.go` — Import 调用 Agent DetectNginx + 解析 + 返回候选站点
- [x] `internal/api/site_handler.go` — GenerateNginx 调 Agent 写配置+校验+reload；ReloadNginx 调 Agent reload
- [x] `internal/api/site_handler.go` — 新增 TakeoverSite handler（接管 candidate → 创建 managed site）
- [x] 编写 handler 测试

### 4. 前端站点页改造
- [x] `web/src/views/Sites.vue` — 双 tab + 服务器筛选 + 导入扫描 + 候选列表 + 接管
- [x] 删除 `web/src/views/NginxImport.vue`
- [x] `web/src/App.vue` — 移除"导入"导航；重写"导入"
- [x] `web/src/router/index.js` — 移除 /import 路由

### 5. 全量验证
- [x] `go test -v -count=1 ./...` 全部通过
- [x] `go build ./...` 编译通过
- [x] `cd web && npm run build` 前端构建通过
