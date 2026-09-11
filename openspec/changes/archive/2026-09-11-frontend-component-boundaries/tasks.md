## 1. Prepare domain component locations

- [x] 1.1 Create `views/monitoring/` and `views/runtime/` directories
- [x] 1.2 Confirm cluster and resources domains can receive terminal components without changing their existing page layout

## 2. Move page-specific components

- [x] 2.1 Move monitoring workspace components and tests into `views/monitoring/`
- [x] 2.2 Move `ChatDrawer.vue` and its test into `views/runtime/`
- [x] 2.3 Move `ServerTerminal.vue` and its test into `views/cluster/`
- [x] 2.4 Move `PodTerminal.vue` into `views/resources/` and add or migrate its component test
- [x] 2.5 Keep `SectionTabsHeader.vue` in `components/` and verify its consumers

## 3. Update imports and verify boundaries

- [x] 3.1 Update page imports and moved component relative imports
- [x] 3.2 Update test imports and mocks for moved components
- [x] 3.3 Search for stale component paths and verify `components/` contains only shared components
- [x] 3.4 Run frontend tests and Vite production build
- [x] 3.5 Run OpenSpec validation and repository-level verification
