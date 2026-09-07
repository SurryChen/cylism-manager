## 1. Release Model and Rendering

- [x] 1.1 添加 `host_network` 的默认值、快照兼容性和资源渲染测试。
- [x] 1.2 在 `ReleaseSpec` 中实现字段，并在 Kubernetes PodSpec 中渲染 `hostNetwork` 和 `ClusterFirstWithHostNet`。
- [x] 1.3 在 `host_network=true` 的 Deployment 上渲染 `Recreate` 策略，并保持非 host-network 和 StatefulSet 行为不变。
- [x] 1.4 已就绪 Pod 忽略已解决的 Warning Event，未就绪 Pod 保留关联诊断。

## 2. Template Editor

- [x] 2.1 在模板编辑表单添加并回显宿主机网络开关。
- [x] 2.2 在模板请求载荷中序列化 `host_network`，并覆盖前端测试。

## 3. Verification

- [x] 3.1 运行相关 Go 与 Vue 测试。
- [x] 3.2 运行 Go 全量测试、构建、Web 测试与构建，并执行安全扫描（前四项已通过；必需的 `/Users/dxm/.security-scan/bin/sec-code` 不存在，宿主机扫描授权亦被拒绝，无法执行扫描）。
