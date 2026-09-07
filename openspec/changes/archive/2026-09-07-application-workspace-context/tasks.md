# Tasks

## 1. Namespace ownership and migration

- [x] 1.1 为 Environment Namespace 实现事务级全局唯一检查、系统 Namespace 拒绝和项目标签验证；先添加 Store/handler 测试。
- [x] 1.2 增加存量重复 Namespace 扫描、冲突 API 与数据库唯一索引延迟创建；测试重复数据不会导致启动失败。
- [x] 1.3 更新环境创建、更新和同步 API，返回可操作的 Namespace 所有权冲突信息。

## 2. Environment-owned resources

- [x] 2.1 为 ManagedDomain 增加 EnvironmentID，迁移可唯一映射的旧记录并标记歧义记录；测试发布拒绝未归属域名。
- [x] 2.2 更新受管域名创建、列表和发布校验，全部以 EnvironmentID 导出 Namespace。
- [x] 2.3 增加应用、发布和资源列表的 project/environment 过滤与摘要 API。
- [x] 2.4 支持将当前环境中的既有单域名 Certificate 导入为非破坏性受管域名资产，并支持关联同命名空间的未归属历史域名记录。
- [x] 2.5 区分 cert-manager 临时 Ready=False 与终态失败，并在签发中时轮询受管域名状态。

## 3. Workspace console

- [x] 3.1 实现浏览器工作上下文状态、URL 同步和项目/环境选择器；覆盖刷新与无上下文状态。
- [x] 3.2 建立工作台、项目与环境、镜像仓库、全局概览二级导航；镜像仓库页统一管理全局仓库和项目授权，项目页仅选择默认仓库。
- [x] 3.3 重构创建应用、发布、受管域名表单，使其从工作上下文推导项目、环境和 Namespace。
- [x] 3.4 展示 Namespace 冲突与旧域名归属迁移状态，并提供迁移操作入口。
- [x] 3.5 为应用详情页提供多模板 CRUD、默认模板选择和版本号发布；发布记录保存模板修订和不可变快照。
- [x] 3.6 将域名从上线模板与发布表单移至应用详情页独立绑定，并在绑定变更时同步 Ingress。

## 4. Verification

- [x] 4.1 执行 Go Store/API/Application 回归测试与新增迁移测试。
- [x] 4.2 执行 `go test ./...`、`go build ./...`、`npm test -- --run`、`npm run build`。
- [x] 4.3 执行 `openspec validate application-workspace-context --strict`，并在真实集群验证 Namespace 归属、受管域名和应用发布。
