# Tasks

- [x] 编写 Kubernetes PVC 客户端测试：创建、托管标签过滤、PV 回收策略/绑定节点映射和删除所有权检查。
- [x] 实现 PVC、PV、StorageClass 只读操作、Environment 解析和 API handler/routes；为输入、跨环境、删除确认与引用拦截编写 API 测试。
- [x] 更新 `k8s/platform-deployment.yaml` 的 ServiceAccount RBAC，加入 PVC 写权限及 PV/StorageClass 最小只读权限。
- [x] 编写 ReleaseSpec/renderer 测试：挂载渲染、重复路径、非法路径、带 PVC 的 Recreate 策略和节点调度约束渲染。
- [x] 扩展 ReleaseSpec、模板请求及发布预检，实现 PVC 归属、Bound 节点、WaitForFirstConsumer、RWO 单副本和节点一致性校验。
- [x] 编写存储卷页面与模板挂载/部署节点编辑器的前端测试。
- [x] 实现存储卷页面、路由和导航；实现模板 PVC 挂载、部署节点选择、创建、删除、回收策略警告与引用提示。
- [x] 运行 `gofmt`、后端 `go test ./...`、`go build ./...`、前端 `npm --prefix web test -- --run` 和 `npm --prefix web run build`。
- [x] 编写并实现 hostname `nodeSelector` 渲染测试：应用 PVC Deployment 与 VictoriaMetrics hostPath Deployment 都不得设置 `PodSpec.NodeName`。
- [x] 编写节点标签 API 测试：查看、自定义标签新增/修改/删除、系统标签保护、非法标签校验和 Kubernetes 错误处理。
- [x] 实现应用与 VictoriaMetrics 的 hostname `nodeSelector` 渲染。
- [x] 实现节点标签 Kubernetes 客户端、HTTP API、路由和集群页标签展示/编辑界面。
- [x] 运行 `gofmt`、后端 `go test ./...`、`go build ./...`、前端 `npm --prefix web test -- --run` 和 `npm --prefix web run build`。
- [x] 完成安全扫描上报，并在验证通过后等待用户确认是否归档该 OpenSpec 变更。
