# Tasks

## API boundary

- [x] 1.1 Inventory direct `api` calls in `Certificates.vue`, `Monitoring.vue`, and `ManagedOCIRegistries.vue`; map each endpoint to one domain API owner without changing its contract.
- [x] 1.2 Add or complete certificate, monitoring, and managed Registry API functions with separate request options and focused API tests.
- [x] 1.3 Replace page-level endpoint construction with domain functions and remove only imports made unused by the migration.

## Request lifecycle and state

- [x] 2.1 Move overlapping certificate, monitoring, and Registry reads onto `useAsyncResource`, forwarding `AbortSignal` through every managed read.
- [x] 2.2 Ensure polling does not overlap, stops on unmount, and retains terminal workflow status.
- [x] 2.3 Split status/query/options/mutation errors and retain the last successful data on later refresh failure.

## Tests and verification

- [x] 3.1 Add deferred-request tests for stale-result protection, abort handling, unmount cleanup, and last-successful-data retention.
- [x] 3.2 Add mutation-failure regression tests for certificate, monitoring, and Registry workflows.
- [x] 3.3 Run frontend tests and build, backend regression tests and build, `openspec validate --all --strict`, and `git diff --check`; report results before archive.
