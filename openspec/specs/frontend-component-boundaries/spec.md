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

### Requirement: Shared component boundary

The frontend SHALL keep only genuinely cross-page components in `web/src/components/`, and SHALL preserve their shared behavior.

#### Scenario: Shared tab header remains reusable

- **WHEN** monitoring, cluster, runtime, or settings pages render their section navigation
- **THEN** they use the same `web/src/components/SectionTabsHeader.vue` implementation

#### Scenario: No page-specific component remains globally owned

- **WHEN** the component directory is inspected after migration
- **THEN** page-specific monitoring, runtime, server-terminal, and Pod-terminal components are absent from `web/src/components/`

### Requirement: Runtime behavior remains stable

The frontend SHALL preserve existing route URLs, API contracts, component behavior, and test coverage after component files are moved.

#### Scenario: Existing pages load moved components

- **WHEN** a user opens monitoring, runtime, server, or workload pages
- **THEN** the same component behavior renders from its new domain-owned location

#### Scenario: Build and tests resolve moved imports

- **WHEN** the frontend test suite and Vite production build run
- **THEN** all moved component, API, composable, utility, and test imports resolve successfully

