# Tasks

## Request and authentication lifecycle

- [x] 1.1 Inventory direct reads in the remaining data-heavy views and add or complete named domain API functions with request options.
- [x] 1.2 Migrate certificates, cluster, sites, resources, services, image registries, DB admin, and system-settings reads to `useAsyncResource` where overlap is possible.
- [x] 1.3 Add single-flight access-token refresh and consistent cancellation/authentication error handling in `web/src/api/index.js`.
- [x] 1.4 Add unit tests for concurrent refresh, refresh failure, retry, cancellation, and stale managed reads.

## Component boundaries

- [x] 2.1 Identify independently testable regions in `Servers.vue`, `Monitoring.vue`, `SystemComponents.vue`, and `Workloads.vue`.
- [x] 2.2 Extract only approved coherent regions, preserving existing props, events, endpoints, payloads, and UI behavior. The SSH terminal is now an isolated `ServerTerminal` boundary; existing monitoring/workload child regions remain intact because they already satisfy this boundary.
- [x] 2.3 Add or update component tests for extracted boundaries and parent orchestration.

## Regression coverage and cleanup

- [x] 3.1 Add focused tests for Login, Certificates, Cluster, Resources, SystemSettings, DBAdmin, and dashboard API behavior.
- [x] 3.2 Add loading, local error, mutation failure, stale-result, and unmount assertions where each workflow supports them.
- [x] 3.3 Audit `web/src` for empty or obsolete artifacts and remove only verified-unused files.

## Bundle and verification

- [x] 4.1 Measure route bundle output and dynamically load optional terminal/chart dependencies where compatible.
- [x] 4.2 Run frontend tests and build, backend regression tests and build, `openspec validate --all --strict`, and `git diff --check`.
- [x] 4.3 Present the completed change for review before archiving the OpenSpec change.
