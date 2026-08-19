## 1. Release Model and Rendering

- [x] 1.1 添加 `host_network` 的默认值、快照兼容性和资源渲染测试。
- [x] 1.2 在 `ReleaseSpec` 中实现字段，并在 Kubernetes PodSpec 中渲染 `hostNetwork` 和 `ClusterFirstWithHostNet`。

## 2. Template Editor

- [x] 2.1 在模板编辑表单添加并回显宿主机网络开关。
- [x] 2.2 在模板请求载荷中序列化 `host_network`，并覆盖前端测试。

## 3. Verification

- [x] 3.1 运行相关 Go 与 Vue 测试。
- [ ] 3.2 运行 Go 全量测试、构建、Web 测试与构建，并执行安全扫描（前四项已通过；必需的 `/Users/dxm/.security-scan/bin/sec-code` 不存在，无法执行扫描）。
