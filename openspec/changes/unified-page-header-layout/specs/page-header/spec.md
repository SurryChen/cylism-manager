# Page Header Layout

## MODIFIED Requirements

### Requirement: Shared page-level headers use a stable visual contract

The frontend SHALL provide `PageHeader` and `SectionTabsHeader` as the only page-level header primitives. Workspace views SHALL use `SectionTabsHeader`, with one tab for a single-view workspace and multiple tabs for sibling views. Detail, edit, terminal, and modal-hosting views without workspace navigation SHALL use `PageHeader`.

#### Scenario: A single-view workspace renders its page header

- **WHEN** a workspace route has no sibling view to switch to
- **THEN** it SHALL render one `SectionTabsHeader` with one active tab, one semantic `h1`, and no route change when that tab is selected

#### Scenario: A multi-view workspace renders its page header

- **WHEN** a workspace route exposes sibling views
- **THEN** it SHALL render one `SectionTabsHeader` with the existing tab state and preserve its current tab selection behavior

#### Scenario: A detail view renders its page header

- **WHEN** a route has no workspace-level tab semantics
- **THEN** it SHALL render one `PageHeader` with optional description and actions without introducing a synthetic tab

#### Scenario: Page headers are rendered across supported viewports

- **WHEN** a page is rendered at desktop or mobile breakpoints
- **THEN** its page-level header SHALL keep the shared height, typography, separator, action alignment, and responsive wrapping tokens without changing the page's data or route behavior

### Requirement: Header adoption preserves view behavior

Views adopting the shared page header contract SHALL preserve their existing routes, API requests, loading, empty, error, filtering, pagination, mutation, and modal behavior.

#### Scenario: A workspace page is migrated

- **WHEN** a workspace view replaces duplicated header markup with `SectionTabsHeader`
- **THEN** its existing content, actions, API calls, and sidebar active state SHALL remain unchanged
