## 1. Header structure

- [x] 1.1 Define the shared primary-header layout tokens and document the standard versus tabbed header contract in the component API.
- [x] 1.2 Align `PageHeader` and `SectionTabsHeader` typography, content separation, action alignment, and responsive behavior without adding a wrapper header.
- [x] 1.3 Add component tests for a single semantic `h1`, action placement, and inline tab structure.

## 2. Records-and-system adoption

- [x] 2.1 Adopt the standard header contract for audit logs and operation history while retaining their data loading, filters, pagination, and actions.
- [x] 2.2 Retain System Settings as one tabbed header and verify its child category views do not introduce another primary header.
- [x] 2.3 Preserve the three direct routes and sidebar active-state behavior; add regression coverage.

## 3. Verification

- [x] 3.1 Run focused component, route, and three-page view tests.
- [x] 3.2 Run full frontend tests and production build.
- [x] 3.3 Run `go test ./...`, `go build ./...`, `openspec validate single-layer-page-header --strict`, and `git diff --check`.
