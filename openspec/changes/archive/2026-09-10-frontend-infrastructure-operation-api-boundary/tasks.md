## API Boundary

- [x] 1.1 Inventory direct infrastructure requests in `Servers.vue`, `Cluster.vue`, and `PersistentVolumes.vue` and map each endpoint to its existing domain API module.
- [x] 1.2 Add server, cluster and storage API functions that preserve HTTP methods, paths, query encoding, payloads, return values, and separate request options.
- [x] 1.3 Add focused API contract tests for all newly owned reads and mutations.

## Request Lifecycle And Errors

- [x] 2.1 Migrate `Servers.vue`, `Cluster.vue`, and `PersistentVolumes.vue` from direct REST calls to named domain API functions.
- [x] 2.2 Use `useAsyncResource` for node and PVC target-dependent reads so stale or unmounted requests cannot update current state.
- [x] 2.3 Scope server, node and storage workflow failures to the initiating form, confirmation, or modal while retaining successful data and retry controls.

## Tests And Verification

- [x] 3.1 Add deferred-request regression tests for target races, abort handling, unmount cleanup, and last-successful-data retention.
- [x] 3.2 Add mutation-failure regression tests for server, node and persistent-volume workflows.
- [x] 3.3 Run frontend tests and build, backend regression tests and build, `openspec validate --all --strict`, and `git diff --check`; report results before archive.
