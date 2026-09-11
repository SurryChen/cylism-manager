## 1. Define contracts with tests

- [x] 1.1 Add `PageHeader.test.js` and `SectionHeading.test.js` covering heading semantics, descriptions, actions slots, and optional back behavior
- [x] 1.2 Add `FeedbackBanner.test.js` and `EmptyState.test.js` covering tone/variant rendering, accessible feedback, loading/no-data messages, and optional action slots
- [x] 1.3 Add `BaseModal.test.js` and `ConfirmDialog.test.js` covering open/close state, Escape and overlay behavior, dialog ARIA attributes, emitted events, and busy confirmation

## 2. Implement shared components

- [x] 2.1 Implement `PageHeader.vue` and `SectionHeading.vue` with slots and Design Token based responsive styles
- [x] 2.2 Implement `FeedbackBanner.vue` and `EmptyState.vue` with accessible variants and action slots
- [x] 2.3 Implement `BaseModal.vue` and `ConfirmDialog.vue` with safe close semantics, dialog labeling, and busy-state protection
- [x] 2.4 Keep `SectionTabsHeader.vue` and all domain-owned monitoring, runtime, terminal, table, and workspace components in their current locations

## 3. Migrate a bounded set of views

- [x] 3.1 Migrate `AuditLogs.vue` to `PageHeader`, `FeedbackBanner`, `EmptyState`, and `BaseModal` while preserving detail-view behavior
- [x] 3.2 Migrate `Applications.vue` to `PageHeader`, `FeedbackBanner`, `EmptyState`, and a confirmation/modal primitive without changing release and project workflows
- [x] 3.3 Migrate `ClusterDNS.vue` or `Configs.vue`, plus `Monitoring.vue` where applicable, to shared feedback, empty, and section-heading primitives
- [x] 3.4 Update or add representative view tests for loading, empty, error, close, cancel, and confirm interactions

## 4. Verify boundaries and regressions

- [x] 4.1 Search for component imports and stale duplicated structures; do not remove shared CSS until all remaining consumers are accounted for
- [x] 4.2 Run the frontend test suite and Vite production build
- [x] 4.3 Run OpenSpec strict validation and repository-level verification before requesting archive review
