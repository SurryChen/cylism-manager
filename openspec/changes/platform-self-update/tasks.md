# Tasks

1. [x] 建立 PlatformRelease、Webhook nonce 数据模型与 Store 测试，覆盖状态转换、过期 nonce 清理和敏感字段不泄露。
2. [x] 编写 HMAC Webhook 鉴权测试，覆盖有效签名、时间漂移、重放、允许镜像前缀与 digest 格式。
3. [x] 实现受限的平台 Deployment 读取/更新与状态收敛测试，验证只能更新 `default/cylism-manager` 的 `platform` 容器。
4. [x] 实现 PlatformRelease 服务、Webhook/管理员 API、启动后状态恢复和人工回滚。
5. [x] 增加系统设置平台自更新界面、发布状态轮询、密钥轮换、镜像前缀配置和回滚确认，并编写前端测试。
6. [x] 更新 GitHub Action：推送成功后取得 digest，按 HMAC 签名调用 `https://cylism.crazycoding.top` 的 Webhook。
7. [ ] 运行 `gofmt`、`go test ./...`、`go build ./...`、`npm --prefix web test -- --run`、`npm --prefix web run build` 和安全扫描上报。
