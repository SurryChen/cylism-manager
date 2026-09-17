## 1. Routed delivery workspaces

- [x] 1.1 Add failing view tests for canonical self-hosted and Proxy query URLs, Tab navigation, and browser history behavior.
- [x] 1.2 Keep `/delivery/registry` as the aggregation entry and preserve the existing `?tab=registry-proxy` URL.
- [x] 1.3 Derive delivery Registry Tab state from the shared routed-Tab composable rather than local hash state.
- [x] 1.4 Run focused router and Registry view tests; present the result before advancing.

## 2. Self-hosted Registry workflow

- [x] 2.1 Add failing Registry workspace tests for grouped overview/catalog actions, a single catalog toolbar boundary, automatic first-repository selection, and fallback selection after repository deletion.
- [x] 2.2 Group service actions with the overview section; add a catalog heading and consolidate its count and search without surrounding separator lines.
- [x] 2.3 Automatically select and load the first available repository when the catalog has no current selection, including after deleting the selected repository.
- [x] 2.4 Run focused managed Registry view tests; present the result before advancing.

## 3. Proxy card and diagnostic consistency

- [x] 3.1 Add failing Registry Proxy workspace tests that assert the shared Registry metric glass-card surface, non-duplicated heading, property-grid separators, and healthy connectivity rendered inside the grid rather than as a red error.
- [x] 3.2 Migrate Proxy instance cards to the shared glass metric surface with scoped layout overrides; replace the outbound cell with structured upstream-connectivity information and render only lifecycle errors as standalone errors.
- [x] 3.3 Run focused Registry Proxy component tests; present the result before advancing.

## 4. Verification

- [x] 4.1 Run the complete frontend test suite and `npm run build`.
- [x] 4.2 Run `git diff --check` and `openspec validate improve-delivery-registry-navigation --strict`.
- [ ] 4.3 Present all verification results and obtain approval before archiving.
