## 1. SelectMenu contract

- [x] 1.1 Add failing `SelectMenu` tests for structured string and numeric values, `v-model.number`, `change`, required validity, disabled state, identifier association, and test attributes.
- [x] 1.2 Extend `SelectMenu` to satisfy the tested value, form, accessibility, and attribute-forwarding contract while retaining the existing Design Token styling.
- [x] 1.3 Run the focused component test suite and verify keyboard opening, Escape, and outside-click behavior.

## 2. Business view migration

- [x] 2.1 Migrate root views `Dashboard`, `ManagedOCIRegistries`, `Monitoring`, `OperationHistory`, and `RuntimeManagement`; preserve page-specific option sources, disabled conditions, and change handlers.
- [x] 2.2 Migrate application views `ApplicationDetails`, `Applications`, `Domains`, `ImageRegistries`, and `ProjectEnvironments`; preserve IDs, ports, protocol values, file-mount selections, and template defaults.
- [x] 2.3 Migrate cluster views `NodeRegistryMirrors`, `PersistentVolumes`, `Servers`, and `SystemComponents`; preserve numeric filters, required form fields, data test IDs, and configuration change handlers.
- [x] 2.4 Migrate monitoring views `AlertingWorkspace`, `DiskGrowthWorkspace`, and `LoggingWorkspace`; preserve numeric runtime, duration, result-limit, project, environment, and application bindings.
- [x] 2.5 Migrate network views `Certificates` and `Sites`; preserve issuer, credential, provider, and namespace selection behavior.
- [x] 2.6 Migrate resource views `Configs`, `PodTerminal`, `Resources`, and `Workloads`; preserve conditional rendering, terminal container selection, explicit label IDs, and inline image-container selection layout.
- [x] 2.7 Migrate settings views `SystemSettingsEntry` and `SystemSettingsSecurity`; preserve explicit label associations, disabled state, and numeric token TTL values.
- [x] 2.8 Migrate the Applications workspace project and environment context menus to `SelectMenu`, preserving project descriptions, environment namespaces, route updates, and disabled state.

## 3. Regression coverage

- [x] 3.1 Update existing frontend tests that query native selects to interact with `SelectMenu` using stable test hooks or its trigger, and add focused page coverage for numeric and change-handler regressions.
- [x] 3.2 Run the complete frontend test suite after the page migration.

## 4. Verification

- [x] 4.1 Confirm `rg --glob '*.vue' '<select\\b' web/src` reports only `web/src/components/SelectMenu.vue`.
- [x] 4.2 Run `npm test` and `npm run build` from `web`.
- [x] 4.3 Run `go test ./...` and `go build ./...` from the repository root.
- [x] 4.4 Run `openspec validate standardize-select-menu --strict` and present the full verification results for user review before archiving.
- [x] 4.5 Re-run full frontend and backend verification after the contextual menu migration, then present the results for user review before archiving.
