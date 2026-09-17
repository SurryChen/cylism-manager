## ADDED Requirements

### Requirement: Shared glass surface component is available

The frontend SHALL provide a reusable `SurfaceCard` component for structural glass panels without owning domain data, loading state, actions, or navigation state.

#### Scenario: A view renders a structural panel

- **WHEN** a view renders `SurfaceCard` with default content
- **THEN** it renders a semantic panel element with the shared glass surface, border, radius, shadow, and theme-aware blur treatment
- **AND** it preserves the supplied content without introducing domain-specific markup

#### Scenario: A view composes a card header and actions

- **WHEN** a view supplies `header` and `actions` slots to `SurfaceCard`
- **THEN** the component renders those regions in a stable card header layout
- **AND** the view remains responsible for the header text and action behavior

### Requirement: Surface card structural variants remain presentation-only

`SurfaceCard` SHALL offer semantic container, padding, and optional interactive presentation variants while keeping interaction behavior under the caller's control.

#### Scenario: A dense information grid needs no outer padding

- **WHEN** a view renders `SurfaceCard` with the no-padding variant
- **THEN** the card retains the shared glass outer surface without adding container padding that disrupts its internal grid separators

#### Scenario: A caller marks a card interactive

- **WHEN** a view renders `SurfaceCard` with its interactive variant
- **THEN** the component exposes the shared hover and focus-visible presentation
- **AND** it does not register click behavior or assign button semantics on behalf of the caller

### Requirement: Registry workspaces adopt the common glass surface without behavior regression

The self-hosted Registry and Registry Proxy workspaces SHALL use `SurfaceCard` for eligible structural panels while preserving their existing workflows and dense data structures.

#### Scenario: A user opens the self-hosted Registry workspace

- **WHEN** a user visits `/delivery/registry` or its self-hosted Registry query-tab URL
- **THEN** the workspace renders its migrated glass panels through `SurfaceCard`
- **AND** existing Registry management, catalog browsing, and repository selection behavior remains unchanged

#### Scenario: A user opens the Registry Proxy workspace

- **WHEN** a user visits `/delivery/registry?tab=registry-proxy`
- **THEN** each migrated Proxy instance retains its property-grid dividers within a shared glass `SurfaceCard`
- **AND** existing diagnostics and lifecycle actions remain unchanged

### Requirement: Records and system panels adopt the common glass surface without behavior regression

The Audit Logs, Operation History, and System Settings pages SHALL use `SurfaceCard` for their existing primary structural panels while retaining their domain workflows.

#### Scenario: A user filters records

- **WHEN** a user searches or filters Audit Logs or Operation History
- **THEN** the existing primary filter and table panel renders through `SurfaceCard`
- **AND** existing filter, pagination, detail, row, and status behavior remains unchanged

#### Scenario: A user opens a system settings tab

- **WHEN** a user opens the security, platform-entry, or release tab at `/settings/system`
- **THEN** that tab's primary settings panel renders through `SurfaceCard`
- **AND** the routed tab, KeepAlive state, forms, and actions remain unchanged
