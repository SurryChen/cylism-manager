## 1. Baseline And Test Contracts

- [x] 1.1 Inventory `Servers.vue` and `Workloads.vue` read resources, mutations, dialogs, polling conditions, and current error ownership without changing behavior.
- [x] 1.2 Add deferred-request tests for server statistics and workload details, including out-of-order resolution and AbortSignal forwarding.
- [x] 1.3 Add regression tests for refresh failure data retention, mutation failure state release, dialog/form preservation, and post-success refresh.
- [x] 1.4 Update `Servers.test.js` and `Workloads.test.js` to mock `api/servers.js` and `api/kubernetes.js` directly, preserving existing assertions.

## 2. Server Page Orchestration

- [x] 2.1 Isolate server list, resource-monitoring, network-diagnostics, and selected-server statistics resources while preserving existing API contracts.
- [x] 2.2 Ensure server save, delete, import, probe, and unbind failures remain local and successful mutations await the affected list refresh.
- [x] 2.3 Ensure resource polling stops on section/document changes and unmount, retains last successful data, and exposes retryable polling errors.
- [x] 2.4 Run the focused `Servers` test suite and verify no stale statistics, disposed updates, or regressions remain.

## 3. Workloads Page Orchestration

- [x] 3.1 Separate inventory loading, keyed workload details, and mutation state without changing resource tabs or existing workflows.
- [x] 3.2 Prevent stale Pod/revision details from overwriting another workload and retain detail context on failed reads.
- [x] 3.3 Give scale, image-update, and rollback operations independent local failure state, preserve dialogs on failure, and await successful inventory refresh.
- [x] 3.4 Run the focused `Workloads` test suite and verify stale detail, mutation retry, and unmount behavior.

## 4. Verification

- [x] 4.1 Run the complete frontend test suite and Vite build.
- [x] 4.2 Run `go test ./...` and `go build ./...` to verify backend compatibility.
- [x] 4.3 Run `openspec validate --all --strict` and `git diff --check`, then report all results before archive.
