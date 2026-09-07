# Tasks

## 1. Domain and certificate model

- [x] 1.1 为 ManagedDomain 增加命名空间、Certificate 与 TLS Secret 标识；为应用入口保存 `domain_id` 与 TLS Secret 引用，并完成 SQLite 迁移。
- [x] 1.2 扩展 K8s Certificate 查询/创建接口，支持按受管域名获取状态与幂等重试；先添加 fake-client 单元测试。
- [x] 1.3 实现域名创建、更新、重试、状态富化和安全删除 API；验证 Namespace、ClusterIssuer Ready、Host 唯一性与引用冲突。

## 2. Application integration

- [x] 2.1 更新发布校验：仅允许使用同命名空间、已就绪的受管域名进行 HTTPS 发布。
- [x] 2.2 更新资源渲染：Ingress 引用域名 TLS Secret，避免重复渲染 Certificate；为 Host + Path 冲突补充校验。
- [x] 2.3 添加 API/application 回归测试，覆盖受管证书复用、路由冲突与域名请求校验。

## 3. Console

- [x] 3.1 重构域名页面为受管 HTTPS 域名流程，加载 Namespace 与 Ready ClusterIssuer 下拉选项。
- [x] 3.2 展示证书状态、Secret、续期、关联入口、DNS 解析说明、重试和签发过程下钻。
- [x] 3.3 更新应用发布弹窗，只列出当前环境中可用于 HTTPS 的受管域名，并展示证书状态。
- [x] 3.4 添加 Vue 测试，覆盖受管域名状态与签发前置条件加载。

## 4. Verification

- [x] 4.1 执行 `go test ./...`、`go build ./...`、`npm test -- --run`、`npm run build`。
- [x] 4.2 执行 `openspec validate managed-domain-lifecycle --strict`。
- [x] 4.3 在真实集群验证 Certificate、TLS Secret 与 Ingress 引用链路。
