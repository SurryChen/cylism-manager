## MODIFIED Requirements

### Requirement: 操作日志记录
系统 SHALL 为长流程操作（部署 Agent、签发证书、续期证书、吊销证书、生成 NGINX 配置、重载 NGINX、导入 NGINX 配置、SSH 连通性测试、应用发布、发布重试和发布回滚）记录每一步的操作日志，包含操作步骤、状态（running/success/failed）、资源类型、资源 ID、详情和时间戳。

#### Scenario: 部署 Agent 操作日志
- **WHEN** 用户对服务器触发 Agent 部署
- **THEN** 系统按顺序写入操作日志：Step 1 "正在连接 SSH" (running)，Step 2 "正在检测操作系统" (running)，Step 3 "正在上传 Agent 二进制" (running)，Step 4 "正在启动 Agent" (running)，各步骤成功后分别更新为 success，若任一步骤失败则更新为 failed 并附加错误详情

#### Scenario: SSH 连通性测试操作日志
- **WHEN** 用户对服务器触发 SSH 连通性测试
- **THEN** 系统写入操作日志：Step 1 "正在测试 SSH 连通性" (running)，连接成功则更新为 success，连接失败则更新为 failed 并附加错误详情

#### Scenario: 应用发布操作日志
- **WHEN** 用户创建、重试或回滚一个 Application Release
- **THEN** 系统以 `resource_type=release` 和 Release ID 写入预检、配置、工作负载、服务、证书、路由和验证步骤的操作日志
- **AND** 日志详情不得包含 Secret 明文

#### Scenario: 操作日志关联资源
- **WHEN** 系统写入操作日志
- **THEN** 每条日志包含 resource_type（如 "server", "cert", "site", "release"）和 resource_id，支持按任意资源类型查询
