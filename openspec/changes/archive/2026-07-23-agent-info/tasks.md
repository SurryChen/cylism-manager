## 实现任务

### 1. Server 模型扩展
- [x] `internal/model/models.go` 新增 `agent_version`、`agent_deploy_path`、`agent_deployed_at` 字段
- [x] 编写模型测试

### 2. 部署与状态同步写入 Agent 信息
- [x] `internal/api/router.go` — DeployAgent 成功后写入 agent 信息；ProbeAgent 成功后若 running 则更新 online
- [x] `internal/api/server_handler.go` — ProbeDeploy 增加状态同步逻辑
- [x] 编写 handler 测试

### 3. 前端 Agent 信息卡片 + 状态同步按钮
- [x] Servers.vue 详情面板新增 Agent 信息卡片
- [x] 新增"状态同步"按钮
- [x] 状态同步操作逻辑

### 4. 全量验证
- [x] `go test -v -count=1 ./...` 全部通过
- [x] `go build ./...` 编译通过
- [x] `cd web && npm run build` 前端构建通过
