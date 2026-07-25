## Why

当前 API 响应格式不一致：有的返回 `{"data": [...], "error": ""}`，有的直接返回裸数组/对象，有的返回 `{"message": "...", "ok": true}`。前端需要针对不同 handler 写不同解析逻辑，且无法从响应结构统一判断请求是否成功。统一返回体可消除解析歧义、简化前端代码、为后续错误码体系打下基础。

## What Changes

- 新增 `APIResponse` 统一响应结构体：`{code: int, message: string, data: interface{}}`，code=0 表示成功
- 新增 `Success()` / `Error()` helper 函数，替换所有 handler 中的裸 `c.JSON()` 调用
- **BREAKING**: 所有 API 响应格式变更，前端需同步适配
- 错误码体系：客户端错误 4xxxx，服务端错误 5xxxx，具体错误分类见 design
- 前端 `api/index.js` 增加统一响应拦截，自动提取 `data` 或抛出错误
- 移除前端各组件中手动的 `d.data || []` / `await r.json()` 判断逻辑，统一由 API 层处理

## Capabilities

### New Capabilities

- `api-response-wrapper`: 统一 API 响应结构体定义、Success/Error helper、错误码枚举

### Modified Capabilities

- `server-management`: 服务器 CRUD 响应格式变更为 APIResponse
- `site-management`: 站点 CRUD 响应格式变更
- `cert-management`: 证书管理响应格式变更
- `dashboard-audit`: Dashboard 和审计日志响应格式变更
- `db-admin`: 数据管理响应格式变更
- `k8s-dashboard`: K8s Dashboard 响应格式变更（已在上一个 change 中扩展）
- `k3s-node`: 节点管理响应格式变更
- `ingressroute-crud`: IngressRoute 管理响应格式变更
- `user-auth`: 认证接口响应格式变更（login/refresh/me）
- `k3s-workload`: 工作负载管理响应格式变更
- `k3s-service-discovery`: 服务发现响应格式变更
- `k3s-config`: 配置管理响应格式变更
- `k3s-ingress-std`: 标准 Ingress 响应格式变更

## Impact

- **后端**: 新增 `internal/model/response.go`（结构体 + helper），修改 11 个 handler 文件共 ~120 处 `c.JSON` 调用，修改 `api/index.js` 前端 API 层
- **前端**: 修改 `web/src/api/index.js` 增加响应拦截，修改 ~8 个 Vue 组件移除手动解析
- **依赖**: 无新增第三方依赖
- **兼容性**: **BREAKING** — 前端和后端必须同步部署
