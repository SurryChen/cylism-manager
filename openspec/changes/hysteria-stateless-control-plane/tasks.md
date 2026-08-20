# Tasks

## 1. Delegated platform access

- [x] 1.1 先添加 auth 单元测试：独立 delegation token 的签名、过期、aud/type、scope 与 action 校验不能接受普通浏览器 JWT。
- [x] 1.2 实现 delegation claims、签发接口与 Integration middleware；为项目、环境和 capability 归属检查添加 Handler 测试。
- [x] 1.3 将 discovery/runtime DTO 复用于 Integration API，添加 delegation 范围、错误脱敏与原始用户审计测试。

## 2. Controlled structured documents

- [x] 2.1 添加 model/store 失败测试，覆盖 managed document 绑定、allowed path 规范化、唯一性与版本并发。
- [x] 2.2 实现表迁移、store 与平台 UI/API 的受控文档创建/查询；校验 application namespace 与精确 Secret/ConfigMap 文件挂载归属。
- [x] 2.3 编写 YAML/JSON patch 的失败测试：禁止根替换、非法指针、范围外路径、版本冲突与敏感响应泄露。
- [x] 2.4 实现服务器端 parse/patch/write 流程及只返回 username 等领域摘要的 Integration endpoint；验证 Secret value 不进入响应、日志或审计。

## 3. Restart release workflow

- [x] 3.1 添加 release service 测试，覆盖受控 patch 后 restart Release、Pod-template 变更与不使用脱敏 DesiredSpec 恢复 Secret。
- [x] 3.2 实现 `restart=true` 与单独 restart Integration API，返回 release ID 并支持受限轮询。
- [x] 3.3 执行所有 Release 旧调用点搜索，添加回归测试保证创建、重试、回滚不受影响。

## 4. Stateless Hysteria Manager

- [x] 4.1 先为平台 HTTP client 写测试，覆盖 delegation forwarding、超时、错误映射和不记录 Authorization/Secret。
- [x] 4.2 删除本地 config/process/state/profile manager，替换为无状态 platform client、签名短期会话与 health check；不引入 SQLite。
- [x] 4.3 重写 Hysteria 应用/用户 handler：发现 Hysteria2、读取用户名摘要、提交 `/auth/userpass` patch、轮询 Release，并只一次性生成 profile。
- [ ] 4.4 更新 Vue UI 与测试，明确没有受控文档授权、发布中、发布失败和无公开 UDP 地址状态。

## 5. Packaging, migration and verification

- [x] 5.1 更新 Hysteria Manager Dockerfile、环境变量示例与 README，移除 Hysteria binary、数据目录、配置文件、流量 API 和本地 profile 文档。
- [x] 5.2 运行 gofmt，完成 `go test ./...`、`go build ./...`、前端单测与构建、OpenSpec strict validation；逐项呈报 task 验证结果。
- [ ] 5.3 对每个修改的业务代码文件完成 `sec-code` 安全扫描并上报风险情况。
- [ ] 5.4 在验证完成、用户确认后归档 OpenSpec 变更并同步基线 spec。
