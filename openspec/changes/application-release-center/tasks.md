## 1. 数据模型与持久化

- [x] 1.1 以 TDD 新增 Project、Environment、Application、ApplicationEndpoint、Release 和 ReleaseOperation GORM 模型及 SQLite 迁移
- [x] 1.2 以 TDD 实现应用领域 Store，覆盖 Project/Environment/Application CRUD、Release 快照、状态转换和上一成功版本查询
- [x] 1.3 为平台管理标签、资源名称和发布定义实现纯函数渲染器，并覆盖输入校验测试

## 2. Kubernetes 发布编排

- [ ] 2.1 扩展 K8s 客户端，创建受管理的 ConfigMap、Secret、Deployment 和 ClusterIP Service，并覆盖标签冲突保护测试
- [ ] 2.2 实现 Deployment/Pod/Endpoint readiness 等待与超时结果采集，并覆盖成功和超时测试
- [ ] 2.3 复用 cert-manager 与 Ingress 能力实现可选 HTTPS 路由，覆盖 Controller/Issuer 不可用和 Certificate 未就绪场景
- [ ] 2.4 实现 Release 编排器、持久化步骤日志、失败重试与从上一成功快照回滚，并覆盖 Secret 缺失阻断场景

## 3. 应用 API 与权限

- [ ] 3.1 实现 Project、Environment、Application 和 Release 的 REST handler 与统一响应测试
- [ ] 3.2 实现发布预检、创建、查询、重试和回滚 API，并覆盖资源冲突、预检失败、发布成功和回滚路径
- [ ] 3.3 将发布步骤写入 OperationLog 和 AuditLog，验证日志不包含 Secret 明文
- [ ] 3.4 更新 K3s RBAC 清单，验证发布所需资源权限最小且完整

## 4. 前端应用发布中心

- [ ] 4.1 新增应用列表和应用详情页面，展示当前 Release、健康摘要、访问地址和发布历史
- [ ] 4.2 新增发布向导，覆盖基本信息、工作负载、配置、服务入口、预检确认和发布进度
- [ ] 4.3 更新导航与路由，新增 `/applications`、`/applications/:id` 和发布入口，并覆盖桌面与移动导航测试
- [ ] 4.4 在现有工作负载、服务、配置、路由和证书页展示所属应用跳转，覆盖平台托管与非托管资源状态

## 5. 验证与交付

- [ ] 5.1 运行 `go test ./...`，覆盖应用发布、K8s 编排、API、Store 和回滚回归测试
- [ ] 5.2 运行 `go build ./...`，确认平台后端可构建
- [ ] 5.3 运行 `cd web && npm test` 与 `cd web && npm run build`，确认发布中心和既有页面无回归
