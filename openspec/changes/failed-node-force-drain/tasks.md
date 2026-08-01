# Tasks

## 1. Kubernetes node state and force drain

- [ ] 1.1 先为 Node Ready Condition 的健康状态派生编写单元测试，覆盖 ready、短暂 not_ready 和心跳超时 failed。
  - 验证：`go test ./internal/k8s -run TestNodeHealthState`
- [ ] 1.2 扩展 `NodeInfo` 与节点列表映射，返回健康状态、原因和最后心跳时间。
  - 验证：K8s fake-client 测试覆盖字段序列化。
- [ ] 1.3 先为故障 worker 强制驱逐编写测试，覆盖 cordon、直接删除受控 Pod、绕过 Eviction API、跳过 DaemonSet/static Pod、阻止无控制器 Pod。
  - 验证：`go test ./internal/k8s -run TestForceDrain`
- [ ] 1.4 实现故障节点强制驱逐及控制面、状态、确认参数的安全校验。
  - 验证：删除动作仅发生在满足全部前置条件时。
- [ ] 1.5 修正普通驱逐错误分类，保留逐 Pod 原始错误和结果。
  - 验证：测试覆盖 PDB 429 与非 PDB 429。

## 2. API and console

- [ ] 2.1 增加 `POST /api/nodes/:id/force-drain`，校验风险确认和节点名，并添加 handler 测试。
  - 验证：`go test ./internal/api -run TestNodeHandler_ForceDrain`
- [ ] 2.2 更新集群节点页面，展示健康状态和普通驱逐逐 Pod 结果。
  - 验证：Vue 测试覆盖 pending 原始原因展示。
- [ ] 2.3 实现故障 worker 强制驱逐对话框，包含影响清单、风险确认、节点名输入和最终结果。
  - 验证：Vue 测试覆盖入口可见性、确认拦截和提交请求。
- [x] 2.4 为已驱逐节点提供状态展示和重新加入操作；重新加入只解除 cordon，不重装或删除节点。
  - 验证：`go test ./internal/k8s ./internal/api`、Vue 集群页测试覆盖状态和请求。

## 3. Verification

## 3. Server binding reconciliation

- [x] 3.1 平台成功移出 Kubernetes Node 后，自动清空绑定服务器的角色和节点名。
  - 验证：`go test ./internal/api -run TestNodeHandler_RemoveNode`
- [x] 3.2 查询服务器时，对成功读取的节点列表执行对账并清理已在集群外删除节点的绑定；K8s API 失败时保留原绑定。
  - 验证：`go test ./internal/api -run TestServerHandlerListUnbinds`
- [x] 3.3 支持手动解除服务器与集群节点绑定，不删除 Kubernetes Node 或工作负载。
  - 验证：`go test ./internal/api -run TestServerHandlerUnbind`、`npm test -- --run Servers.test.js`

## 4. Interactive terminal reliability

- [x] 4.1 用成熟 WebSocket 库替换手写帧解析，并为大文本粘贴添加回归测试。
  - 验证：`go test ./internal/api -run TestWSConnReadFrameHandlesLargeTerminalPaste`
- [x] 4.2 修复 SSH 和 Pod 终端的遮罩关闭、剪贴板粘贴和白色终端画布。
  - 验证：`npm test -- --run`、`npm run build`

## 5. Verification

- [x] 5.1 执行 `go test ./...`、`go build ./...`、`npm test -- --run`、`npm run build`。
- [x] 5.2 执行 `openspec validate failed-node-force-drain --strict`。
- [ ] 5.3 在真实集群使用已失联的 worker 验证：安全驱逐保留 PDB、强制驱逐只删除受控 Pod、Node 移出仍需人工确认，服务器卡片恢复为未加入集群。
