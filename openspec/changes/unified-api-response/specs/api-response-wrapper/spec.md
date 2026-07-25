## ADDED Requirements

### Requirement: 统一响应结构体
系统 SHALL 定义 `APIResponse` 结构体，包含 `code`(int)、`message`(string)、`data`(interface{}) 三个字段。

#### Scenario: 成功响应格式
- **WHEN** handler 返回成功
- **THEN** 响应 JSON 为 `{"code": 0, "message": "ok", "data": <payload>}`

#### Scenario: 错误响应格式
- **WHEN** handler 返回错误
- **THEN** 响应 JSON 为 `{"code": <error_code>, "message": "<描述>", "data": null}`

### Requirement: Success helper
系统 SHALL 提供 `Success(c *gin.Context, data interface{})` 函数，自动包装成功响应。

#### Scenario: 返回列表数据
- **WHEN** 调用 `Success(c, []Item{...})`
- **THEN** HTTP 200 + `{"code": 0, "message": "ok", "data": [...]}`

#### Scenario: 返回单个对象
- **WHEN** 调用 `Success(c, item)`
- **THEN** HTTP 200 + `{"code": 0, "message": "ok", "data": {...}}`

### Requirement: Error helper
系统 SHALL 提供 `Error(c *gin.Context, httpStatus int, code int, msg string)` 函数，自动包装错误响应。

#### Scenario: 参数错误
- **WHEN** 调用 `Error(c, 400, 40001, "invalid params")`
- **THEN** HTTP 400 + `{"code": 40001, "message": "invalid params", "data": null}`

#### Scenario: 资源不存在
- **WHEN** 调用 `Error(c, 404, 40401, "not found")`
- **THEN** HTTP 404 + `{"code": 40401, "message": "not found", "data": null}`

### Requirement: 错误码定义
系统 SHALL 在 `response.go` 中集中定义所有错误码常量。

#### Scenario: 错误码唯一性
- **WHEN** 新增错误码
- **THEN** 必须在 `response.go` 的 const 块中定义，避免与其他 handler 冲突

### Requirement: 前端统一响应拦截
前端 `api/index.js` SHALL 提供 `_unwrapResponse()` 方法，自动检查 `code === 0`。

#### Scenario: 成功响应自动解包
- **WHEN** API 返回 `{"code": 0, "data": [...]}`
- **THEN** 调用方直接获得 `[...]` 数据

#### Scenario: 错误响应自动抛异常
- **WHEN** API 返回 `{"code": 40001, "message": "参数错误", "data": null}`
- **THEN** `_unwrapResponse` 抛出 Error("参数错误")，调用方可 catch
