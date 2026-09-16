## ADDED Requirements

### Requirement: Route views use one primary header layer

The web application SHALL render exactly one primary page-header component for each route view. A cross-route sidebar MUST remain the owner of route navigation and MUST NOT be duplicated as a page-content header or tab container.

#### Scenario: Open an untabbed records route

- **WHEN** a user opens audit logs or operation history from the records-and-system sidebar
- **THEN** the selected route renders one `PageHeader` with its own title, optional description, and optional actions
- **AND** it does not render an additional records-domain header or page-level tab strip

#### Scenario: Open System Settings

- **WHEN** a user opens System Settings from the records-and-system sidebar
- **THEN** the route renders one `SectionTabsHeader` containing the System Settings title and its settings-category tabs
- **AND** the active settings-category content does not render another primary page header

### Requirement: Primary header variants have stable shared layout behavior

The web application SHALL use shared tokens and component styles for standard and tabbed primary headers, including consistent page-content separation, title typography, action alignment, and responsive behavior.

#### Scenario: Render a header with actions

- **WHEN** a route renders a primary header with contextual statistics or actions
- **THEN** the actions remain within the same header layer and do not create a second title row

#### Scenario: Use a narrow viewport

- **WHEN** a primary header is rendered on a narrow viewport
- **THEN** its title, actions, and optional tabs remain readable and accessible without overlapping or introducing another navigation layer

### Requirement: Existing records routes remain direct navigation targets

The web application SHALL preserve the independent records-and-system route targets and their sidebar active states.

#### Scenario: Navigate between records routes

- **WHEN** a user selects Audit Logs, Operation History, or System Settings in the sidebar
- **THEN** the application navigates directly to `/audit`, `/operations`, or `/settings/system` respectively
- **AND** only the matching sidebar item is active
