## 1. Test and Theme Foundation

- [x] 1.1 Add a Vite-compatible frontend test runner and write failing tests for saved palette restoration, system-preference fallback, and immediate palette switching.
- [x] 1.2 Add `lucide-vue-next` and verify the production build resolves the icon dependency.
- [x] 1.3 Create semantic palette token, glass fallback, reduced-motion, and shared component stylesheet modules for all four palettes.
- [x] 1.4 Implement the palette Composition API helper and pre-mount root attribute application until the theme tests pass.

## 2. Application Shell and Navigation

- [x] 2.1 Write failing component tests for desktop side navigation, mobile drawer routing, and palette menu accessibility.
- [x] 2.2 Rebuild the authenticated App shell with grouped desktop side navigation, compact top bar, Lucide controls, and selected route states.
- [x] 2.3 Implement the responsive mobile drawer, keyboard-safe close behavior, and mobile palette selector until navigation tests pass.
- [x] 2.4 Verify all existing routes remain reachable and no obsolete import navigation item is rendered.

## 3. Shared Glass Components

- [x] 3.1 Migrate shared buttons, form controls, filters, tabs, data tables, status badges, empty states, overlays, and modals to semantic Glass UI classes.
- [x] 3.2 Remove global and view-level hard-coded color values and replace inline visual styles with reusable semantic classes.
- [x] 3.3 Add tests or browser assertions for opaque fallback surfaces, keyboard focus, and reduced-motion behavior.

## 4. Page Migration

- [x] 4.1 Redesign Login and Dashboard with Pastel Glass hierarchy while preserving authentication and dashboard API loading behavior.
- [x] 4.2 Redesign Servers and Routes pages while preserving server, node, cluster, route filters, dialogs, and mutations.
- [x] 4.3 Redesign Certificates and Resources pages while preserving namespace filters, certificate lifecycle actions, resource tabs, and service details.
- [x] 4.4 Redesign Audit Logs and Data Management pages while preserving pagination, table selection, record editing, and delete confirmation behavior.
- [x] 4.5 Run page-level frontend tests across all four palettes and confirm every route's primary action remains visible at desktop and 375px widths.

## 5. Verification

- [x] 5.1 Run `npm run build` in `web` and fix all frontend build failures.
- [x] 5.2 Run `go test ./...` and `go build ./...` to confirm the UI rewrite does not regress the full project.
- [x] 5.3 Perform desktop and mobile visual regression checks for Mint Glass, Mist Blue, Orchid Glass, and Night Glass, including glass fallback and reduced-motion states.

## 6. Workbench Stability, Reference Glass, and Sky Veil Palette

- [x] 6.1 Add failing tests for the Sky Veil palette and root-layer palette menu rendering.
- [x] 6.2 Add the screenshot-inspired Sky Veil semantic token set and persist it through the existing palette preference helper.
- [x] 6.3 Rebuild the desktop shell as a fixed independent workbench card with a separate content scroll plane and non-reflowing active navigation state.
- [x] 6.4 Render the desktop palette menu with `Teleport`, fixed anchored positioning, and viewport-safe placement.
- [x] 6.5 Restructure the dashboard into a compact metrics strip and asymmetric main work area while preserving its data requests and empty states.
- [x] 6.6 Write failing assertions for the unframed desktop top bar, two Direction 05 geometric background bands, and Reference Glass token values.
- [x] 6.7 Align the desktop top bar, geometric background bands, Reference Glass palette label/tokens, and shared glass surface treatment until task 6.6 passes.
- [x] 6.8 Perform a manual browser-layout pass across all five palettes.
- [x] 6.9 Write failing component tests for top-level navigation, route-derived secondary navigation, and mobile full-navigation preservation.
- [x] 6.10 Rework the application shell into a full-width Direction 05 primary navigation bar with route-derived secondary side navigation until task 6.9 passes.
