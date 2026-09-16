# frontend-shared-ui-components Specification

## Purpose
TBD - created by archiving change frontend-shared-ui-components. Update Purpose after archive.
## Requirements
### Requirement: Shared page structure components are available

The frontend SHALL provide reusable `PageHeader` and `SectionHeading` components for page-level and section-level headings without taking ownership of business state or data fetching.

#### Scenario: A page renders a title and actions

- **WHEN** a view renders `PageHeader` with a title and an actions slot
- **THEN** the component SHALL render one semantic page heading and expose the actions in a stable region without requiring page-specific header markup

#### Scenario: A section renders supporting text

- **WHEN** a view renders `SectionHeading` with a title, description, and optional actions
- **THEN** the component SHALL render the title, supporting text, and actions while leaving the section's data and layout ownership to the view

### Requirement: Shared feedback and empty states are consistent

The frontend SHALL provide `FeedbackBanner` and `EmptyState` components with theme-aware variants, accessible semantics, and optional action content.

#### Scenario: A view reports an error or warning

- **WHEN** a view renders `FeedbackBanner` with an error, warning, success, or informational tone
- **THEN** the component SHALL expose the corresponding visual variant and accessible feedback semantics without changing the supplied message

#### Scenario: A list has no data or is still loading

- **WHEN** a view renders `EmptyState` for loading, no results, or an actionable empty condition
- **THEN** the component SHALL render a consistent message area and only render the optional action slot when provided

### Requirement: Modal primitives preserve safe interaction

The frontend SHALL provide `BaseModal` and `ConfirmDialog` primitives that expose explicit open/close state, accessible dialog semantics, and safe confirmation actions.

#### Scenario: A modal is opened and dismissed

- **WHEN** a view opens `BaseModal`
- **THEN** the modal SHALL render an overlay and dialog with `aria-modal="true"`, support the configured close behavior, and emit a close event without mutating the view's business state

#### Scenario: A destructive action requires confirmation

- **WHEN** a view opens `ConfirmDialog` and the user confirms or cancels
- **THEN** the component SHALL emit the corresponding event, preserve the supplied title/message, and disable repeated confirmation while `busy` is true

### Requirement: Shared components follow project visual and accessibility conventions

Shared components SHALL use existing Design Tokens and remain compatible with the project's responsive Glass UI styles and reduced-motion preferences.

#### Scenario: A shared component is rendered under a different palette

- **WHEN** any shared component renders under the configured mint, blue, orchid, sky, or night palette
- **THEN** it SHALL use the existing semantic CSS variables and remain readable without hard-coded palette-specific colors

#### Scenario: A user prefers reduced motion

- **WHEN** the system enables `prefers-reduced-motion`
- **THEN** shared component transitions SHALL be reduced or disabled without removing content or preventing keyboard interaction

### Requirement: Adoption does not change page behavior

Views adopting shared components SHALL preserve their existing routes, API requests, business state, error messages, and domain-specific behavior.

#### Scenario: A representative page is migrated

- **WHEN** Audit Logs, Applications, Cluster DNS/Configs, or Monitoring adopts a shared component
- **THEN** the page SHALL retain its existing loading, empty, error, submit, cancel, and navigation behavior while replacing only duplicated presentation structure

#### Scenario: A shared component is not genuinely cross-page

- **WHEN** a component depends on monitoring, runtime, terminal, workload, or another single domain's business model
- **THEN** it SHALL remain colocated with that owning view instead of being added to `web/src/components/`

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

