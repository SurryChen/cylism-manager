## Why

Operational pages still mix direct `api` calls with domain modules and often share one loading or error state across unrelated regions. During fast tab, filter, pagination, or route changes, a slower response can overwrite newer state, while mutation and polling failures are either hidden or shown in the wrong region. This change makes the high-frequency operational pages predictable and testable without changing backend endpoints.

## What Changes

- Move Workloads, Configs, AuditLogs, and NodeRegistryMirrors reads and mutations behind named domain API functions.
- Use `useAsyncResource` for overlapping page reads, including abort, latest-result-wins protection, and unmount cleanup.
- Add explicit local error and loading states for list, detail, mutation, and apply-progress polling regions; ignored `AbortError` must not be shown as a user failure.
- Preserve the existing REST paths, payloads, page layouts, and user-visible workflows.
- Consolidate duplicate API ownership, including node reads currently duplicated between `cluster.js` and `system-components.js`.
- Add focused API and view tests for request encoding, stale-result protection, unmount behavior, pagination, detail loading, mutations, and polling failures.
- Review `ChatDrawer.vue` and extract only stable, independently testable regions such as session or approval lists; keep stream orchestration in the parent.

## Capabilities

### New Capabilities

- `frontend-operational-api-ownership`: Named, domain-owned API functions for operational pages and a single owner for shared endpoint definitions.

### Modified Capabilities

- `frontend-resource-request-lifecycle`: Extend cancellable, latest-result-wins reads and local error handling to Workloads, Configs, AuditLogs, and NodeRegistryMirrors, and cover polling and mutation state.

## Impact

- Frontend views: `web/src/views/Workloads.vue`, `Configs.vue`, `AuditLogs.vue`, `NodeRegistryMirrors.vue`, and selected stable regions of `ChatDrawer.vue`.
- Frontend API modules under `web/src/api/`, plus `web/src/composables/useAsyncResource.js` only if a small compatibility improvement is required.
- Existing view and API tests; no backend handlers, response schemas, or deployment manifests.
- No new runtime dependency, global store, Pinia module, or query library.
