## 1. Test and Theme Foundation

- [ ] 1.1 Add a Vite-compatible frontend test runner and write failing tests for saved palette restoration, system-preference fallback, and immediate palette switching.
- [ ] 1.2 Add `lucide-vue-next` and verify the production build resolves the icon dependency.
- [ ] 1.3 Create semantic palette token, glass fallback, reduced-motion, and shared component stylesheet modules for all four palettes.
- [ ] 1.4 Implement the palette Composition API helper and pre-mount root attribute application until the theme tests pass.

## 2. Application Shell and Navigation

- [ ] 2.1 Write failing component tests for desktop side navigation, mobile drawer routing, and palette menu accessibility.
- [ ] 2.2 Rebuild the authenticated App shell with grouped desktop side navigation, compact top bar, Lucide controls, and selected route states.
- [ ] 2.3 Implement the responsive mobile drawer, keyboard-safe close behavior, and mobile palette selector until navigation tests pass.
- [ ] 2.4 Verify all existing routes remain reachable and no obsolete import navigation item is rendered.

## 3. Shared Glass Components

- [ ] 3.1 Migrate shared buttons, form controls, filters, tabs, data tables, status badges, empty states, overlays, and modals to semantic Glass UI classes.
- [ ] 3.2 Remove global and view-level hard-coded color values and replace inline visual styles with reusable semantic classes.
- [ ] 3.3 Add tests or browser assertions for opaque fallback surfaces, keyboard focus, and reduced-motion behavior.

## 4. Page Migration

- [ ] 4.1 Redesign Login and Dashboard with Pastel Glass hierarchy while preserving authentication and dashboard API loading behavior.
- [ ] 4.2 Redesign Servers and Routes pages while preserving server, node, cluster, route filters, dialogs, and mutations.
- [ ] 4.3 Redesign Certificates and Resources pages while preserving namespace filters, certificate lifecycle actions, resource tabs, and service details.
- [ ] 4.4 Redesign Audit Logs and Data Management pages while preserving pagination, table selection, record editing, and delete confirmation behavior.
- [ ] 4.5 Run page-level frontend tests across all four palettes and confirm every route's primary action remains visible at desktop and 375px widths.

## 5. Verification

- [ ] 5.1 Run `npm run build` in `web` and fix all frontend build failures.
- [ ] 5.2 Run `go test ./...` and `go build ./...` to confirm the UI rewrite does not regress the full project.
- [ ] 5.3 Perform desktop and mobile visual regression checks for Mint Glass, Mist Blue, Orchid Glass, and Night Glass, including glass fallback and reduced-motion states.

## 6. Workbench Stability, Reference Glass, and Sky Veil Palette

- [x] 6.1 Add failing tests for the Sky Veil palette and root-layer palette menu rendering.
- [x] 6.2 Add the screenshot-inspired Sky Veil semantic token set and persist it through the existing palette preference helper.
- [x] 6.3 Rebuild the desktop shell as a fixed independent workbench card with a separate content scroll plane and non-reflowing active navigation state.
- [x] 6.4 Render the desktop palette menu with `Teleport`, fixed anchored positioning, and viewport-safe placement.
- [x] 6.5 Restructure the dashboard into a compact metrics strip and asymmetric main work area while preserving its data requests and empty states.
- [ ] 6.6 Write failing assertions for the unframed desktop top bar, two Direction 05 geometric background bands, and Reference Glass token values.
- [ ] 6.7 Align the desktop top bar, geometric background bands, Reference Glass palette label/tokens, and shared glass surface treatment until task 6.6 passes.
- [ ] 6.8 Perform a manual browser-layout pass across all five palettes.
- [ ] 6.9 Write failing component tests for top-level navigation, route-derived secondary navigation, and mobile full-navigation preservation.
- [ ] 6.10 Rework the application shell into a full-width Direction 05 primary navigation bar with route-derived secondary side navigation until task 6.9 passes.
