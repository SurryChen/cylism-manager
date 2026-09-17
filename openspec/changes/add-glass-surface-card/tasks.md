## 1. Shared component (TDD)

- [x] 1.1 Add failing `SurfaceCard` tests for semantic container selection, default/header/actions slots, padding variants, and the optional interactive state.
- [x] 1.2 Implement `SurfaceCard` using existing Glass UI design tokens and reduced-motion-compatible styles; do not introduce domain state or event behavior.
- [x] 1.3 Run the focused component test and verify the component preserves slot content under each structural variant.

## 2. Registry adoption (TDD)

- [x] 2.1 Add or update failing Registry workspace tests asserting that their structural glass panels use `SurfaceCard` while business content and actions remain unchanged.
- [x] 2.2 Migrate eligible self-hosted Registry overview/catalog panels and Proxy instance panels to the shared component; retain the Proxy property-grid separators and existing data-table layouts.
- [x] 2.3 Run focused Registry component/view tests and verify routed tabs, repository selection, and Proxy diagnostics retain their behavior.

## 3. Delivery component boundary (TDD)

- [x] 3.1 Add or update failing imports/tests that expect `RegistryProxyWorkspace` and its test to live in `web/src/views/delivery/`, not `web/src/components/`.
- [x] 3.2 Move the Proxy workspace and colocated test into the delivery view domain; update the parent Registry workspace import without changing domain behavior.
- [x] 3.3 Run the focused delivery Registry tests and verify no component-directory import remains for `RegistryProxyWorkspace`.

## 4. Records and system card adoption (TDD)

- [x] 4.1 Add failing Audit Logs and Operation History tests that assert their primary filter/table panels use `SurfaceCard` while preserving search, filter, detail, and pagination behavior.
- [x] 4.2 Migrate the Audit Logs and Operation History primary panels to `SurfaceCard`; retain their local layout classes only for content structure.
- [x] 4.3 Add failing System Settings tests that assert each active settings tab renders its primary settings panel through `SurfaceCard` while preserving routed tabs and KeepAlive state.
- [x] 4.4 Migrate Security, Entry, and Release settings panels to `SurfaceCard`; retain their forms and actions unchanged.
- [x] 4.5 Run focused records-and-system tests and verify table rows are not converted into individual cards.

## 5. Verification

- [x] 5.1 Run the complete frontend test suite and `npm run build`.
- [x] 5.2 Run `go test ./...`, `go build ./...`, `openspec validate add-glass-surface-card --strict`, and `git diff --check`.
- [ ] 5.3 Present verification results and obtain approval before archiving.
