# Design: 平台自更新

## Network and trust boundary

GitHub Actions 是唯一主动方：镜像构建并推送到阿里云镜像仓库后，Runner 请求 `POST https://cylism.crazycoding.top/api/platform/deployments`。Cylism 不调用 GitHub API，也不需要到 GitHub 的出口网络。K3s 节点只需要具备到镜像仓库的拉取网络。

管理员在系统设置生成一次部署密钥，将其作为 GitHub Repository Secret `CYLISM_DEPLOY_WEBHOOK_SECRET` 保存。平台只保存该密钥的加密值，设置页仅在生成时展示一次。请求使用以下头：

- `X-Cylism-Timestamp`：Unix 秒，允许正负 5 分钟。
- `X-Cylism-Nonce`：至少 16 字符的随机值，24 小时内只能使用一次。
- `X-Cylism-Signature`：`HMAC-SHA256(secret, timestamp + "." + nonce + "." + rawBody)` 的十六进制值。

签名使用 constant-time 比较。时间戳、nonce 和签名错误统一返回未授权；已成功接受的 nonce 写入 SQLite，避免 GitHub 请求被捕获后重放。Webhook 路由不使用 JWT，但仍经专用 Webhook 鉴权中间件；常规 UI 与状态接口继续使用 JWT。

## Release model

新增 `PlatformRelease` 表：`id`、`source`、`image`、`previous_image`、`status`、`detail`、`requested_at`、`started_at`、`completed_at`。状态为 `accepted`、`applying`、`waiting_ready`、`succeeded`、`failed`、`rolled_back`。镜像只允许完整 digest 引用，且必须匹配管理员配置的平台仓库前缀；拒绝 tag-only、无 digest、空值和不匹配仓库。

提交成功后立即创建记录并返回 `202 Accepted`，再更新唯一允许的目标：Namespace `default` 中、名称 `cylism-manager` 的 Deployment 内名称为 `platform` 的容器。更新时写入 `cylism.io/platform-release=<id>` 和 `cylism.io/platform-restarted-at` Pod Template Annotation，触发滚动更新。请求在 Deployment 更新前完成持久化，因此 HTTP 连接因自身重启中断时仍可恢复状态。

新实例启动和每次状态查询都会读取最新未完成记录与 Deployment 状态：目标容器 image/digest 匹配且 Deployment 的 `AvailableReplicas >= desired` 时标记成功；出现 ReplicaFailure、进度超时或镜像不一致时标记失败。平台不在旧 Pod 内阻塞等待就绪。状态页通过短轮询读取该持久化记录。

管理员可从系统设置发起回滚，只允许回滚到记录中的 `previous_image`。回滚同样创建一个审计化 PlatformRelease，并使用上述同一渲染和状态收敛逻辑。

## Image resolution

GitHub Action 通过 build-push-action 的 digest 输出构造 `${image}@${digest}` 并提交。平台仅做格式、允许前缀与 Kubernetes 更新校验，不在自更新路径主动访问 GitHub。可选的镜像 Manifest 远程验证仅在平台可访问镜像仓库时执行，验证失败不更新 Deployment；若网络条件不具备，Action 已完成仓库推送而 K3s 的 ImagePull 状态仍是最终事实来源。

## UI and workflow

系统设置新增“平台自更新”卡片，显示：当前 Deployment image、最近发布、状态和错误详情。管理员可生成/轮换 Webhook 密钥、配置镜像仓库前缀、复制 Action 所需 Secrets 名称和工作流片段，并从历史记录发起回滚。

工作流在 `push` 到 `main` 且镜像推送成功后，使用 `curl --fail-with-body` 向公开 Webhook 发送 JSON：`image`、`commit_sha`、`run_id`。Action 只等待接受响应，不等待平台重启完成；平台 UI 提供状态查看。

## RBAC

现有 ServiceAccount 对 Deployment 的 `get`、`update`、`patch` 已满足本功能，不增加集群范围权限。

## Risks and mitigations

- 公开部署接口被滥用：HMAC、时间窗口、nonce、防重放、镜像仓库允许列表和固定目标 Deployment 共同约束。
- 上传可运行但不存在的镜像：仅接受 Action 返回的 digest；Kubernetes ImagePullBackOff 会被状态收敛标记为失败。
- 自身重启丢失进度：发布先落库，新 Pod 持续收敛未完成记录。
- 错误升级：保存 previous image，并将回滚保留为人工确认动作。
