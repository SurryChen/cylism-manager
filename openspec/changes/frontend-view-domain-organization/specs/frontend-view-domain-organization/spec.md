## ADDED Requirements

### Requirement: Domain-organized view sources

The frontend SHALL organize route view source files and their colocated tests into a small, explicit set of domain directories without changing route behavior.

#### Scenario: Application views are grouped

- **WHEN** a maintainer looks for application workspace, project, release, registry, or domain pages
- **THEN** the related Vue files and their tests are located under `web/src/views/applications/`

#### Scenario: Infrastructure views are grouped

- **WHEN** a maintainer looks for cluster, Kubernetes resource, or network pages
- **THEN** the related Vue files and their tests are located under the corresponding `cluster/`, `resources/`, or `network/` directory

#### Scenario: Settings views are grouped

- **WHEN** a maintainer looks for system settings pages or their shared settings stylesheet
- **THEN** the related files are located under `web/src/views/settings/`

### Requirement: Runtime behavior remains stable

The frontend SHALL preserve existing route URLs, lazy-loading behavior, API contracts, and rendered page behavior after the view files are moved.

#### Scenario: Existing routes continue to resolve

- **WHEN** the application navigates to an existing route
- **THEN** the same page component loads from its new source location without a route URL change

#### Scenario: Frontend build resolves moved imports

- **WHEN** the Vite production build runs
- **THEN** all moved view, component, composable, stylesheet, and test imports resolve successfully
