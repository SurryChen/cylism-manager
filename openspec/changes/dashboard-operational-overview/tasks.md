## 1. Backend dashboard data correctness

- [x] 1.1 Add repository/service tests for future-only certificate expiry and separate expired certificate counting.
- [x] 1.2 Update dashboard read model and handler error handling so optional certificate and audit failures are explicit instead of silently becoming empty results.
- [x] 1.3 Add tests for dashboard response fields, preview-count semantics, and section-level error payloads.

## 2. Kubernetes summary service

- [x] 2.1 Add tests proving Pod readiness uses `PodReady=True`, not only `PodPhase=Running`.
- [x] 2.2 Add tests for mixed node versions and the resulting inconsistent-version status.
- [x] 2.3 Introduce a composed Kubernetes dashboard summary service with a narrow read interface, explicit partial errors, and context propagation.
- [x] 2.4 Add a bounded TTL cache and concurrent refresh coalescing; test cache hit, expiry, refresh failure, and cancellation behavior.
- [x] 2.5 Update the Kubernetes handler and route wiring to use the composed summary service while preserving existing metric fields and API paths.

## 3. Dashboard frontend state and semantics

- [x] 3.1 Add Dashboard tests for loading, success, empty, section failure, and partial Kubernetes summary states.
- [x] 3.2 Update Dashboard data mapping and labels to distinguish unavailable values, expired/expiring certificates, unready Deployments, and limited recent-operation previews.
- [x] 3.3 Add tests that high-priority items link to alert, certificate, workload, and audit detail pages without stale or misleading values.

## 4. Compact first-screen layout

- [x] 4.1 Refactor Dashboard preview sections to cap certificate and recent-operation rows at three and provide “查看全部” links.
- [x] 4.2 Remove fixed empty-state heights and implement the compact desktop two-column layout with a single-column mobile fallback.
- [x] 4.3 Add source/layout assertions for high-frequency action entries and the first-screen content-density constraints.

## 5. Verification

- [x] 5.1 Run focused Go tests for dashboard and Kubernetes summary behavior.
- [x] 5.2 Run the full Go test suite and `go build ./...`.
- [x] 5.3 Run the full frontend test suite and `npm run build`.
- [x] 5.4 Run `git diff --check` and `openspec validate dashboard-operational-overview`.
