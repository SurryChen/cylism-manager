## 1. Managed Logging Resources

- [x] 1.1 Define Loki/Alloy configuration, status, validation and infrastructure PVC ownership constants with focused unit tests.
- [x] 1.2 Create/reconcile Loki StatefulSet, Service, PVC and retention ConfigMap with readiness/status tests.
- [x] 1.3 Create/reconcile Alloy ServiceAccount, least-privilege RBAC, ConfigMap and DaemonSet with tests for read-only host-path mounts and label relabeling.
- [x] 1.4 Implement safe uninstall and configuration update behavior that preserves the Loki PVC and rejects unsupported storage relocation, with regression tests.
- [x] 1.5 Extend infrastructure storage inventory and generic-operation protection for Loki-owned PVCs, with API and K8s tests.

## 2. Logging APIs

- [x] 2.1 Add authenticated status, install, uninstall and configuration handlers with request validation and route tests.
- [x] 2.2 Add structured filter-option and bounded Loki-query handlers, including scope validation, LogQL escaping/construction, upstream timeout and error mapping tests.
- [x] 2.3 Register monitoring log API routes and verify JWT protection plus rejection of raw LogQL and oversized requests.
- [x] 2.4 Support exact UTC time bounds and bounded quoted AND/OR keyword expressions, including safe Loki branch construction and result deduplication tests.

## 3. Monitoring Logs Workspace

- [x] 3.1 Add the Logs top-level monitoring tab with install, loading, unavailable and ready summary states, preserving lazy tab loading.
- [x] 3.2 Implement hierarchical structured filters, time-range selection, keyword search and reverse-chronological log viewer with component tests.
- [x] 3.3 Add centered Logs settings modal for retention configuration and show managed PVC navigation/status without exposing internal endpoints.
- [x] 3.4 Add precise local time controls and configurable returned-line count to the Logs query workspace, with component tests.

## 4. Verification

- [x] 4.1 Run focused Go and Vue tests while completing each task, then run `go test ./...`, `go build ./...`, `npm --prefix web test`, `npm --prefix web run build`, `openspec validate monitoring-logs --strict`, and `git diff --check`.
