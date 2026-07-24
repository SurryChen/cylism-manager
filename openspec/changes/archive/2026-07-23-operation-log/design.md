## 背景

平台当前 AuditLog 只记录操作结果（谁在什么时候做了什么），是粗粒度的审计。但 deploy / issue-cert 等操作耗时 10-30 秒且包含多步，用户需要知道"卡在哪一步了"。OperationLog 填补这个空白，记录操作过程的每一步状态。

## 目标 / 非目标

**目标：**
- 8 类长流程操作每一步产生一条日志（running → success/failed）
- 通用 API 按资源类型+ID 查询，适配 server/site/cert 等多种资源
- 配置化自动清理（默认 30 天）
- 前端服务器详情面板实时展示日志（轮询）

**非目标：**
- 操作日志的 WebSocket 实时推送（一期用轮询）
- 日志导出/下载
- 操作日志的搜索和全文索引

## 技术决策

### 1. 通用资源关联（resource_type + resource_id）

与 AuditLog 保持一致模式，用 `resource_type` + `resource_id` 替代单一 `server_id`。

- 备选：每类资源单独建表 — 表膨胀，查询复杂

### 2. 日志写入：同步回调

`deployServiceImpl` 和后续 service 内部调用 `addLog(resourceType, resourceID, ...)`。

- 备选：channel 异步写入 — 过度设计，日志量极小（每次操作 3-5 条）

### 3. 清理策略

后台 goroutine，每小时检查一次。配置在 config.yaml：

```yaml
operation_log:
  retention_days: 30  # 0 = 永不过期
```

### 4. 前端轮询

`setInterval` 2 秒。仅在有 running 状态的日志时启动，全部完成或关闭面板时停止。

## 风险与权衡

- **[日志膨胀]** 频繁操作可能产生大量日志 → 可配置清理策略，默认 30 天
- **[轮询开销]** 每 2 秒一次 HTTP 请求 → 仅在详情面板打开时轮询，关闭即停止
