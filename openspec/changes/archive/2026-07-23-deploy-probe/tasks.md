## 实现任务

### 1. Agent 新增 --version flag
- [x] 在 `cmd/agent/main.go` 中注册 `--version` flag
- [x] 实现版本号常量（如 `version = "1.0.0"`）
- [x] 编写 `--version` 输出的单元测试

### 2. SSH Client 新增 ProbeAgent 方法
- [x] 在 `internal/service/deployer/ssh_client.go` 中新增 `ProbeAgent()` 方法
- [x] 依次执行：检测进程(ps)、systemd 状态、二进制文件存在性、版本号
- [x] 返回 `AgentProbeResult` 结构体
- [x] 编写 `ProbeAgent` 的单元测试（mock SSH）

### 3. DeployService 接口改造
- [x] `DeployService` 接口新增 `ProbeAgent(server) (*AgentProbeResult, error)`
- [x] `DeployAgent` 方法新增 `force bool` 参数
- [x] `deployServiceImpl` 实现 force 模式：先 stop 旧 Agent，再覆盖部署
- [x] 编写接口层单元测试

### 4. 新增探测与改造部署 Handler
- [x] 新增 `POST /api/servers/:id/deploy/probe` 路由
- [x] 新增 `ProbeDeploy` handler
- [x] 改造 `Deploy` handler 支持 `?force=true` 查询参数
- [x] 编写 handler 单元测试

### 5. 前端探测-确认-执行交互
- [x] 点击"部署"按钮先调 probe 接口（按钮 loading 态）
- [x] 弹窗展示探测结果（无 Agent / 有 Agent 区分）
- [x] 确认后调 deploy?force=true，取消则关闭弹窗
- [x] 部署中显示进度状态

### 6. 全量验证
- [x] `go test -v -count=1 ./...` 全部通过
- [x] `go build ./...` 编译通过
- [x] `cd web && npm run build` 前端构建通过
