## 1. Node registry mirror adoption (TDD)

- [x] 1.1 Add a failing `NodeRegistryMirrors` test asserting that the mirror-rule workspace uses `SurfaceCard` while its table and operational controls remain present.
- [x] 1.2 Migrate the mirror-rule workspace to `SurfaceCard` with a slot-based heading; preserve its table structure, dialogs, polling and action handlers.
- [x] 1.3 Run the focused node-mirror view test and inspect its desktop and narrow viewport layout.

## 2. Legacy content-panel inventory and migration (TDD)

- [x] 2.1 Search all legacy `.card`, `.card-header` and `.card-title` call sites; classify content panels by domain and explicitly exclude modal, banner, metric and per-record elements.
- [x] 2.2 Add or update failing view tests for the overview and application-domain content panels, then migrate their eligible outer containers to `SurfaceCard` without changing data, routes or actions.
- [x] 2.3 Add or update failing view tests for cluster and resource-domain content panels, then migrate their eligible outer containers to `SurfaceCard` without changing data, routes or actions.
- [x] 2.4 Add or update failing view tests for network, monitoring and runtime-domain content panels, then migrate their eligible outer containers to `SurfaceCard` without changing data, routes or actions.
- [x] 2.5 Run focused tests after each domain batch and confirm no new content panel introduces a legacy `.card` outer container.

## 3. Legacy style retirement (TDD)

- [x] 3.1 Add a failing style or source-level test proving migrated panels no longer require the `.card` outer-surface class and unrelated surfaces retain their visual rules.
- [x] 3.2 Split the global combined selector as needed and remove the legacy `.card` glass-surface and mobile-padding declarations; retain metric, modal and banner styling.
- [x] 3.3 Search all frontend call sites to confirm there are no eligible content-panel `.card` consumers and no stale selectors tied to removed markup.

## 4. Verification

- [x] 4.1 Run focused component and domain-view tests, then `npm test` and `npm run build`.
- [x] 4.2 Run `go test ./...`, `go build ./...`, `openspec validate migrate-legacy-card-surfaces --strict`, and `git diff --check`.
- [x] 4.3 Present the verification results and obtain approval before archiving.
