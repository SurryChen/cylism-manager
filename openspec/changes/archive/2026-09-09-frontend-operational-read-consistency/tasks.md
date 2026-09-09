## 1. API Modules And Contracts

- [x] 1.1 Add or extend Kubernetes workload/config API functions with optional request options and exact path/query contract tests.
- [x] 1.2 Add `web/src/api/audit.js` with canonical pagination, filter, sort, and order encoding plus API tests.
- [x] 1.3 Add `web/src/api/node-registry-mirrors.js` covering mirrors, proxies, diagnostics, apply, and apply-status calls plus API tests.
- [x] 1.4 Consolidate duplicate shared endpoint definitions, especially node reads, and verify all imports with `rg`.

## 2. Workloads And Configs

- [x] 2.1 Write Workloads tests for cancellable inventory reads, stale-result protection, unmount disposal, and local list errors.
- [x] 2.2 Migrate Workloads reads to domain APIs and `useAsyncResource`, including expanded pod detail reads.
- [x] 2.3 Write Workloads mutation tests covering scale, image update, rollback failure, submitting reset, and visible mutation errors.
- [x] 2.4 Implement Workloads mutation state and refresh behavior without changing existing payloads or layout.
- [x] 2.5 Write Configs tests for namespace/tab races, detail failures, save/delete failures, and preservation of list data.
- [x] 2.6 Migrate Configs list/detail reads and create/edit/delete operations to domain API functions with separate list/detail/mutation state.

## 3. Audit And Node Registry Mirrors

- [x] 3.1 Write AuditLogs tests for encoded filters, pagination, cancellation on filter changes, retained data on failure, and visible local errors.
- [x] 3.2 Migrate AuditLogs to `audit.js` and `useAsyncResource` while preserving current pagination and detail behavior.
- [x] 3.3 Write NodeRegistryMirrors tests for independent mirror/proxy/list errors, mutation failures, apply progress, polling failure, and unmount cleanup.
- [x] 3.4 Migrate NodeRegistryMirrors calls to its API module and introduce lifecycle-safe polling with separate error states.

## 4. ChatDrawer Boundary Review

- [x] 4.1 Inspect current ChatDrawer tests and select the smallest stable session or approval presentation boundary.
- [x] 4.2 Record the decision to retain the current parent boundary because session controls and stream state are tightly coupled; no artificial child extraction was made.
- [x] 4.3 Verify stream, retry, approval, and session orchestration behavior remains in the parent and existing tests continue to pass.

## 5. Verification

- [x] 5.1 Run the focused frontend API, composable, and view tests for every completed task.
- [x] 5.2 Run the full frontend test suite and `vite build`.
- [x] 5.3 Run `go test ./...`, `go build ./...`, `openspec validate --all --strict`, and `git diff --check`.
