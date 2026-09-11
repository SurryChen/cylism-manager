## API Boundary

- [x] 1.1 Inventory direct monitoring API calls in `AlertingWorkspace.vue`, `LoggingWorkspace.vue`, and `DiskGrowthWorkspace.vue` and map each endpoint to `alerting.js`, `logging.js`, or `monitoring.js`.
- [x] 1.2 Add named domain API functions that preserve current HTTP methods, paths, query encoding, payloads, return values, and separate request options.
- [x] 1.3 Add focused API tests for read signal forwarding and mutation contracts.

## Request Lifecycle And State

- [x] 2.1 Move disk-growth and workspace-dependent reads to `useAsyncResource`; cancel or ignore stale reads on selection changes and unmount.
- [x] 2.2 Keep alert status, log status/filter options, query results, and mutation state independently refreshable without clearing successful data.
- [x] 2.3 Split status, filter/options, query, and mutation errors into their smallest applicable UI regions.

## Tests And Verification

- [x] 3.1 Add deferred-request regression tests for stale responses, abort handling, unmount cleanup, and last-successful-data retention.
- [x] 3.2 Add mutation-failure regression tests for alerting, logging, and diagnostic workflows.
- [x] 3.3 Run frontend tests and build, backend regression tests and build, `openspec validate --all --strict`, and `git diff --check`; report results before archive.
