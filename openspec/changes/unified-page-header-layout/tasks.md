## 1. Shared header contract

- [ ] 1.1 Finalize shared page-header tokens for height, typography, separator, spacing, and responsive behavior.
- [ ] 1.2 Update `PageHeader` and `SectionTabsHeader` to implement the shared contract without owning page business state.
- [ ] 1.3 Add component tests for semantic heading count, stable dimensions, action placement, single-tab behavior, and reduced-motion compatibility.

## 2. Workspace page migration

- [ ] 2.1 Migrate existing workspace/Hub pages to `SectionTabsHeader`, using a single tab where a page has no sibling workspace view.
- [ ] 2.2 Remove duplicated Hub header markup and page-level title CSS after each migration.
- [ ] 2.3 Keep detail, edit, terminal, and modal-hosting pages on `PageHeader` where no workspace tab semantics exist.
- [ ] 2.4 Preserve routes, sidebar active states, API requests, filters, pagination, actions, and error/empty/loading states.

## 3. Regression coverage

- [ ] 3.1 Update representative view tests for all migrated workspace pages.
- [ ] 3.2 Add a route/navigation regression test covering single-tab and multi-tab pages.
- [ ] 3.3 Verify desktop and mobile header layout at the supported breakpoints.

## 4. Verification

- [ ] 4.1 Run focused frontend component and view tests after each migration batch.
- [ ] 4.2 Run full frontend tests and production build.
- [ ] 4.3 Run `go test ./...`, `go build ./...`, `openspec validate unified-page-header-layout --strict`, and `git diff --check`.
