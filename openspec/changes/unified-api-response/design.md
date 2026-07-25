## Context

项目当前有 163 处 `c.JSON()` 调用分布在 11 个 handler 文件中，响应格式至少存在三种不同形态：`{data, error}`、裸对象/数组、`{message, ok}`。前端 `api/index.js` 无响应拦截，每个 Vue 组件自行解析 JSON 并处理错误。

## Goals / Non-Goals

**Goals:**
- 定义统一的 `APIResponse{Code, Message, Data}` 结构体
- 提供 `Success()` / `Error()` helper，替换所有 handler 中的裸 `c.JSON`
- 前端 API 层增加响应拦截，统一提取 `data` 或抛出错误
- 错误码体系：4xxxx 客户端错误，5xxxx 服务端错误

**Non-Goals:**
- 不引入 gin middleware 自动包装（避免 hack ResponseWriter）
- 不修改 gRPC Agent 通信协议
- 不修改 `cmd/` 入口文件

## Decisions

### 决策 1: Helper 函数而非中间件

**选择**: 用 `Success(c, data)` / `Error(c, httpStatus, code, msg)` helper，handler 显式调用。

**备选**: gin middleware 自动包装。  
**取舍**: middleware 需要 wrap `ResponseWriter` 截获输出，复杂度高且 gin 对 `c.Writer.Written()` 的行为不够可靠。helper 方式侵入性虽高（~120 处改动），但行为明确、易调试、无隐式 magic。

### 决策 2: 错误码体系

**选择**: 数字错误码，按模块分段。

| 范围 | 含义 |
|------|------|
| 0 | 成功 |
| 40001-40099 | 通用客户端错误（参数校验、未授权等） |
| 40100-40199 | 认证相关 |
| 40400-40499 | 资源不存在 |
| 50001-50099 | 通用服务端错误 |
| 50100-50199 | K8s 集群相关错误 |

**备选**: 字符串错误码（如 `"INVALID_PARAM"`）。  
**取舍**: 数字码前端 switch 更高效，且与 HTTP status code 可对应（40xxx → 4xx）。

### 决策 3: 前端统一拦截

**选择**: 在 `web/src/api/index.js` 的 `api` 对象中增加 `_unwrapResponse()`，自动检查 `code === 0`，非 0 抛出 Error。

```js
async _unwrapResponse(res) {
  const json = await res.json()
  if (json.code !== 0) throw new Error(json.message)
  return json.data
}
```

**备选**: Vue composable 或在每个组件中处理。  
**取舍**: API 层统一拦截后，所有调用方直接拿到 `data` 字段，无需手动 `d.data || []`、无需 try/catch 检查。

### 决策 4: 认证接口特殊处理

**选择**: login/refresh/me 接口也使用统一格式，但前端需特殊处理 401 响应。

**备选**: 认证接口保持独立格式。  
**取舍**: 统一格式有利于前端统一拦截；login 返回的 token 放在 `data.access_token` 中。

## Risks / Trade-offs

- **[BREAKING]** 全量响应格式变更，前后端必须同步部署 → 部署时先更新后端再更新前端，确保向后兼容
- **[遗漏风险]** ~120 处 `c.JSON` 改动，可能遗漏 → 通过 `rg` 遍历所有 `c.JSON` 调用确保全部替换
- **[前端回归]** 前端 ~8 个组件需同步修改解析逻辑 → 逐个组件验证，vitest 全量跑通
- **[错误码冲突]** 多人开发时错误码可能重复 → 集中定义在 `response.go` 的 const 块中
