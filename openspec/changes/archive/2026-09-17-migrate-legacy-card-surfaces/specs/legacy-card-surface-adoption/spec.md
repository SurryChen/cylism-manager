## ADDED Requirements

### Requirement: Content panels use the shared surface component

The frontend SHALL render eligible page-level and section-level content panels through `SurfaceCard` rather than the legacy `.card` outer-surface class.

#### Scenario: A user opens the node registry mirror workspace

- **WHEN** a user visits `/cluster?tab=registry-mirrors`
- **THEN** the mirror-rule workspace renders through `SurfaceCard`
- **AND** the rule table, verification, selected-node application, records, inspection, restart and delete workflows retain their existing behavior

#### Scenario: A user opens a migrated operational page

- **WHEN** a user opens a page whose legacy content panel has been migrated
- **THEN** its page-level or section-level content surface renders through `SurfaceCard`
- **AND** its existing route, data requests, empty states, tables, forms and actions remain owned by that page

### Requirement: Card headers retain readable structure at responsive widths

The frontend SHALL compose card-level headings and action controls through the shared card header regions without obscuring content at supported widths.

#### Scenario: A migrated panel has a heading and actions

- **WHEN** a page supplies a heading and actions to a migrated card
- **THEN** the heading and actions render in the `SurfaceCard` header and actions regions
- **AND** action controls may wrap at a narrow viewport without overlapping the heading or panel content

### Requirement: Legacy card surface styles are removed only after migration

The frontend SHALL remove the global `.card` outer-surface contract after all eligible content panels have adopted `SurfaceCard`, while preserving unrelated surface types.

#### Scenario: Shared component styles are inspected after migration

- **WHEN** the shared stylesheet is inspected after the migration
- **THEN** it does not define the legacy `.card` glass surface or `.card` mobile padding rule
- **AND** metric, modal and warning-banner styling remains available without relying on `.card`
