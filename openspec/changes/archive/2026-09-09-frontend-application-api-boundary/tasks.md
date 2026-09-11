# Tasks

## API boundary

- [x] 1.1 Inventory every application-related direct `api` call in `Applications.vue`, `ApplicationDetails.vue`, and `ReleaseDetails.vue`; map each call to a named domain function without changing its contract.
- [x] 1.2 Add or complete application domain API functions and focused API tests for paths, payloads, URL encoding, and request options.
- [x] 1.3 Replace the inventoried view-level endpoint construction with domain API calls and remove only imports made unused by the migration.

## Request lifecycle and state

- [x] 2.1 Move route-, selection-, and filter-dependent application reads onto `useAsyncResource`, passing `signal` through every managed read.
- [x] 2.2 Ensure release/detail polling and component disposal cannot update state after selection changes or unmount.
- [x] 2.3 Split list/detail/mutation errors where they currently overwrite each other, preserving existing user-facing messages.

## Tests and verification

- [x] 3.1 Add deferred-request tests for stale-result protection, abort handling, unmount cleanup, and last-successful-data retention.
- [x] 3.2 Add mutation-failure regression tests for application creation, project/template/endpoint changes, release actions, and workload-kind changes.
- [x] 3.3 Run frontend tests and build, backend regression tests and build, `openspec validate --all --strict`, and `git diff --check`; report results before archive.
