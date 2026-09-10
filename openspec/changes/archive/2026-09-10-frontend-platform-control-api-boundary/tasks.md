## 1. API Contracts

- [x] 1.1 Inventory direct control-plane REST calls in system components, system settings subpages, and Cluster DNS, mapping each to its domain API module.
- [x] 1.2 Add named system component, platform settings, temporary-token and Cluster DNS functions that preserve methods, paths, encoding, payloads, unwrapped values and optional request options.
- [x] 1.3 Add focused API contract tests for all newly owned reads and mutations.

## 2. Page Lifecycle And Errors

- [x] 2.1 Migrate `SystemComponents.vue`, `SystemSettingsEntry.vue`, `SystemSettingsRelease.vue`, `SystemSettingsSecurity.vue`, and `ClusterDNS.vue` to named domain API functions.
- [x] 2.2 Use `useAsyncResource` for overlapping control-plane reads and cancel or ignore stale and unmounted requests.
- [x] 2.3 Keep control-plane mutation failures in their initiating modal, form, confirmation or tab region while retaining successful data and retry controls.

## 3. Tests And Verification

- [x] 3.1 Add deferred-request regression tests for control-plane stale reads, abort/unmount cleanup and last-successful-data retention.
- [x] 3.2 Add mutation-failure regression tests for component, settings and Cluster DNS workflows.
- [x] 3.3 Run frontend tests and build, backend regression tests and build, `openspec validate --all --strict`, and `git diff --check`; report results before archive.
