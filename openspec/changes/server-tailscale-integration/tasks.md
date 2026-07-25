## 1. 数据库变更

- [x] 1.1 `internal/model/models.go` — Server 删除 Status/LastSeen/Sites 字段，新增 TailscaleIP/TailscaleOnline
- [x] 1.2 `internal/model/models.go` — 新增 SystemConfig 模型
- [x] 1.3 `internal/store/store.go` — 新增 system_configs 表自动迁移，新增 GetSystemConfig/SetSystemConfig 方法
- [x] 1.4 `internal/store/store.go` — DashboardStats 删除 OnlineServers，GetDashboardStats 移除 status="online" 查询
- [x] 1.5 `internal/store/store_test.go` — 修复因 Server 字段变更导致的编译错误和断言失败

## 2. 后端：Tailscale 集成

- [x] 2.1 新增 `internal/api/tailscale_handler.go` — Init/Status handler
- [x] 2.2 `internal/api/router.go` — 注册 /api/tailscale 路由组
- [x] 2.3 新增 `scripts/install-tailscale.sh` — 一键安装 Tailscale 脚本
- [x] 2.4 `internal/api/tailscale_handler_test.go` — Init/Status 测试

## 3. 后端：SSH 连通性检测

- [x] 3.1 `internal/api/server_handler.go` — 新增 Probe handler（POST /api/servers/:id/probe）
- [x] 3.2 `internal/api/router.go` — 注册 /api/servers/:id/probe 路由
- [x] 3.3 连通性检测逻辑：解密 SSH 凭据 → 尝试连接（5s 超时）→ 返回结果

## 4. 后端：前置检测

- [x] 4.1 `internal/api/server_handler.go` — 新增 Precheck handler（POST /api/servers/:id/precheck）
- [x] 4.2 实现 5 项检测：SSH 连接 / root 权限 / swap 状态 / OS 兼容性 / 磁盘空间
- [x] 4.3 `internal/api/router.go` — 注册 /api/servers/:id/precheck 路由

## 5. 后端：WebSocket 进度日志

- [x] 5.1 新增依赖 `github.com/gorilla/websocket`
- [x] 5.2 `internal/api/node_handler.go` — 新增 JoinProgress WS handler
- [x] 5.3 实现 12 步流程完整逻辑（前置检测 + Tailscale + k3s-agent）
- [x] 5.4 `internal/api/router.go` — 注册 WS /api/nodes/:id/join-progress 路由
- [x] 5.5 `internal/api/node_handler_test.go` — 新增 WS handler 测试

## 6. 后端：Server handler 适配

- [x] 6.1 `internal/api/server_handler.go` — Create 删除 Status:"offline"
- [x] 6.2 `internal/api/server_handler_test.go` — 同步适配

## 7. 前端：Servers.vue 重构

- [x] 7.1 表格新增列：SSH 用户、认证方式、SSH 连通性（🔍按钮）、TS IP、TS 状态
- [x] 7.2 SSH 连通性：点击 🔍 → POST /api/servers/:id/probe → 弹窗展示结果
- [x] 7.3 加入集群按钮：点击 → POST /api/servers/:id/precheck → 前置检测弹窗
- [x] 7.4 前置检测弹窗：展示 5 项检测结果，全部通过后显示「确认加入」
- [x] 7.5 确认加入 → 建立 WebSocket → 进度日志弹窗（实时滚动 + 步骤图标）
- [x] 7.6 `web/src/views/Servers.test.js` — 适配测试

## 8. 前端：新组件

- [x] 8.1 新增 `web/src/components/ProgressModal.vue` — 可复用的进度日志弹窗组件
- [x] 8.2 新增 `web/src/views/TailscaleConfig.vue` — Tailscale 配置页面
- [x] 8.3 `web/src/router/index.js` — 添加 Tailscale 配置路由

## 9. 前端：Dashboard 适配

- [x] 9.1 `web/src/views/Dashboard.vue` — 删除「在线」指标卡片
- [x] 9.2 `web/src/views/Dashboard.vue` — 新增 Tailscale 未初始化 Banner（检测 /api/tailscale/status）
- [x] 9.3 `web/src/views/Dashboard.test.js` — 适配测试

## 10. 部署配置

- [x] 10.1 `k8s/platform-deployment.yaml` — 新增 ENCRYPTION_KEY 环境变量

## 11. 全量验证

- [x] 11.1 `go test ./...` 全部通过
- [x] 11.2 `go build ./...` 编译通过
- [x] 11.3 `npx vitest run` 前端测试通过
- [x] 11.4 `npx vite build` 构建成功
