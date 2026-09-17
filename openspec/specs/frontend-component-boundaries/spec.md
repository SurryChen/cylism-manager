# frontend-component-boundaries Specification

## Purpose
TBD - created by archiving change frontend-component-boundaries. Update Purpose after archive.
## Requirements
### Requirement: Domain-owned page components

Page-specific frontend components SHALL live next to the view domain that owns their behavior, together with their colocated tests.

#### Scenario: Monitoring components are colocated

- **WHEN** a maintainer looks for alerting, logging, disk-growth, or metric-trend implementation
- **THEN** the component and its test are located under `web/src/views/monitoring/`

#### Scenario: Runtime and infrastructure components are colocated

- **WHEN** a maintainer looks for Agent chat, server terminal, or Pod terminal implementation
- **THEN** the component and its test are located under the owning `runtime/`, `cluster/`, or `resources/` view directory

#### Scenario: Delivery workspace is colocated

- **WHEN** a maintainer looks for Registry Proxy workspace behavior
- **THEN** `RegistryProxyWorkspace.vue` and its test are located under `web/src/views/delivery/`
- **AND** they are not located in `web/src/components/`

### Requirement: Shared component boundary

The frontend SHALL keep only genuinely cross-page components in `web/src/components/`, and SHALL preserve their shared behavior.

#### Scenario: Shared tab header remains reusable

- **WHEN** monitoring, cluster, runtime, or settings pages render their section navigation
- **THEN** they use the same `web/src/components/SectionTabsHeader.vue` implementation

#### Scenario: No page-specific component remains globally owned

- **WHEN** the component directory is inspected
- **THEN** a component that owns a single domain's API requests, route state, or operational actions is absent from `web/src/components/`

#### Scenario: Shared card remains generic

- **WHEN** a view renders `web/src/components/SurfaceCard.vue`
- **THEN** the component provides only the structural glass surface and slots
- **AND** it does not import Registry Proxy APIs, domain workspaces, or other domain-specific modules

### Requirement: Runtime behavior remains stable

The frontend SHALL preserve existing route URLs, API contracts, component behavior, and test coverage after component files are moved.

#### Scenario: Delivery Registry loads its moved workspace

- **WHEN** a user opens `/delivery/registry?tab=registry-proxy`
- **THEN** the Registry Proxy workspace renders from the delivery view domain
- **AND** its existing API requests, diagnostics, forms, and lifecycle actions continue to work

#### Scenario: Build and tests resolve moved imports

- **WHEN** the frontend test suite and Vite production build run
- **THEN** all moved component, API, composable, utility, and test imports resolve successfully

