# Design: Frontend Page State and Request Handling

## Request lifecycle

Use the existing `web/src/composables/useAsyncResource.js` for page-owned reads that can overlap because of route, filter, tab, or pagination changes. Add named functions to `web/src/api/` for direct endpoint calls before migrating a view. A managed read must pass its `AbortSignal`, cancel the previous read, ignore responses from older reads, retain the last successful data on later failure, and stop updating after scope disposal.

The migration starts with `Certificates.vue`, `Cluster.vue`, `Sites.vue`, `Resources.vue`, `Services.vue`, `ImageRegistries.vue`, `DBAdmin.vue`, and the system-settings views. Static hub or redirect views only need a managed resource if they own data fetching.

## Authentication refresh

Keep token storage in the existing API module. When multiple requests receive HTTP 401, the first request creates the refresh Promise and later requests await it. A successful refresh updates both tokens before original requests retry. A failed refresh clears the session and performs one redirect to login. Request errors expose enough context for pages to distinguish cancellation, authentication failure, HTTP failure, and business failure while preserving the current user-facing messages.

## Component boundaries

Split only complete UI regions from large views. Candidate boundaries include the server resource panel, server editor and terminal overlay; monitoring trend/workload regions; system-component detail/operation areas; and workload detail panels. The parent view retains route state, API orchestration, mutation commands, and cross-region coordination. Extracted components receive explicit props and emit events rather than importing page-local state.

## Testing and bundle work

Add focused tests beside the affected source files. Prioritize login/session transitions, certificate and cluster mutations, resources loading failures, settings saves, database administration actions, and dashboard API functions. Tests must cover loading, failure, stale-result, unmount, and mutation-failure behavior where applicable.

Optional terminal and chart dependencies may be dynamically imported at the point of use, provided route behavior and error handling remain unchanged. Remove only files confirmed to have no references, such as empty placeholders.
