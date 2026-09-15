## ADDED Requirements

### Requirement: Compact audit filters
The audit page MUST keep keyword, outcome, and resource filters in the primary toolbar and expose lower-frequency filters in a modal.

#### Scenario: Open advanced filters
- **WHEN** the user activates the filter button
- **THEN** source, action, actor, target, and date fields are shown in a modal without expanding the result table layout

#### Scenario: Apply and clear filters
- **WHEN** the user applies advanced filters
- **THEN** the audit request contains the selected values and active conditions are visible as removable chips
- **WHEN** the user resets filters
- **THEN** all filter values are cleared and the first page is loaded

### Requirement: Auditable time range
The audit list endpoint MUST support inclusive start-date and exclusive end-date filtering while preserving existing query parameters.

#### Scenario: Date range query
- **WHEN** the request includes `created_from=2026-09-01` and `created_to=2026-09-08`
- **THEN** records from September 1 inclusive through before September 9 are returned

### Requirement: Shared select presentation
The web application MUST provide a reusable custom select component with consistent visual, focus, disabled, keyboard, and listbox behavior.

#### Scenario: Select an option
- **WHEN** the user opens a custom select and chooses an option
- **THEN** the component emits the selected value and closes the option list
