## Design: Frontend Monitoring Workspace API Boundary

## Technical Decisions

### API ownership follows the workspace domain

- `alerting.js` owns告警安装、状态、概览、静默、自动处置、通知测试和设置。
- `logging.js` owns日志安装、状态、筛选项、查询、保留策略更新和卸载。
- `monitoring.js` owns磁盘增长诊断读取，因为该端点与 VictoriaMetrics 监控查询属于同一基础监控域。

Each function accepts business payload and request options as separate arguments. It preserves the existing method, route, query-name encoding and unwrapped response exactly.

### Request lifecycle stays local

`useAsyncResource` remains the single local lifecycle primitive. It cancels a prior request before a new refresh, ignores stale completions through its request ID, and cancels on component unmount. No global store or cache is introduced.

- Disk range/node changes refresh one resource; the prior result remains visible while the current request runs.
- Logging refresh, filter options and explicit query remain separate resources so a failed filter refresh cannot erase log results.
- Alert status and ready-state overview remain separate resources; settings-only reads must not block the active-alert view.

### Errors match the affected region

State errors appear around the workspace status. Filter/options failures appear near their selectors. Query failures appear near the query result. Mutation errors appear in the operation surface that initiated them. Abort errors remain silent. A failed refresh never resets prior successful state or result lists.

### Test strategy

Add focused domain API tests for paths, methods, payloads, query encoding and signal propagation. Use deferred promises in component tests to assert stale disk/log responses cannot overwrite newer selections, pending reads are aborted on unmount, prior successful data is retained after a failure, and unsuccessful mutations keep their forms/results available.

## Alternatives Considered

### A single `observability.js` API file

Rejected: alerting, logging and base monitoring already have distinct resource lifecycles. Preserving domain modules keeps ownership discoverable and avoids one broad file.

### One shared workspace error

Rejected: a log query failure must not hide a healthy Loki status or valid filters, and a notification test failure must not invalidate active alerts.

### Add a new polling abstraction

Rejected: no new periodic workflow is needed in these workspaces. Existing `useAsyncResource` and the parent monitoring lifecycle cover current requirements.

## Risks and Mitigations

- **Risk:** extraction changes an existing mutation payload. **Mitigation:** API contract tests assert exact calls and page tests retain the existing interaction assertions.
- **Risk:** cancellation leaves a workspace in an incorrect loading state. **Mitigation:** reuse the tested `useAsyncResource` cleanup behavior and add component-level unmount assertions.
- **Risk:** local errors become invisible. **Mitigation:** retain existing Chinese fallback text and position messages in the corresponding status, options, query, or action region.
