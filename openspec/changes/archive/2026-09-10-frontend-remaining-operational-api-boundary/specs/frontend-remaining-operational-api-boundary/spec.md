## ADDED Requirements

### Requirement: Remaining operational endpoints have named domain ownership

Domain, route, image registry, Chart repository and project environment pages SHALL use named functions from their domain API modules rather than construct their operational REST paths in the view.

#### Scenario: An operator manages an operational resource

- **WHEN** an operator creates, updates, validates, synchronizes or deletes a managed domain, route, image registry, Chart repository or project environment
- **THEN** the view SHALL invoke a named domain API function that preserves the existing HTTP method, path, encoded identifiers, payload and unwrapped response

#### Scenario: Request options are supplied to a domain function

- **WHEN** a page supplies an AbortSignal to a managed read
- **THEN** the domain API function SHALL forward it separately from the business parameters without changing the request contract

### Requirement: Project environment reads are current and cancellable

Project environment data that depends on the active project route SHALL cancel or ignore superseded requests and SHALL only commit the active route result.

#### Scenario: An operator changes projects during a pending read

- **WHEN** the active project changes before the earlier project environment read settles
- **THEN** the earlier result SHALL NOT replace projects, applications, conflicts, environments or loading state for the newer project

#### Scenario: The project environment view is removed during a read

- **WHEN** the project environment view unmounts while a managed read is pending
- **THEN** its request SHALL be cancelled or ignored and SHALL NOT update disposed state or show an abort error

### Requirement: Operational mutation failures remain actionable and non-destructive

A rejected route, repository, domain or project environment mutation SHALL remain visible in its smallest applicable page region and SHALL retain the relevant form, confirmation and successful data for retry.

#### Scenario: A route operation fails

- **WHEN** creating or deleting an IngressRoute or Ingress is rejected
- **THEN** `Sites.vue` SHALL show a local error instead of swallowing the failure and SHALL keep the relevant dialog or loaded route data available

#### Scenario: A repository or environment operation fails

- **WHEN** a Chart repository, image registry or project environment mutation is rejected
- **THEN** the initiating form or confirmation SHALL remain usable, its entered values or target SHALL remain available, and unrelated loaded data SHALL not be cleared

### Requirement: Remaining operational workflows have focused regression coverage

The frontend SHALL provide API contract and view regression tests for remaining operational API ownership, project-switch request isolation and local mutation errors.

#### Scenario: A future endpoint refactor changes a request contract

- **WHEN** a named domain API function changes its path, method, encoded identifier, payload or optional request options
- **THEN** the focused API contract tests SHALL detect the regression

#### Scenario: A high-risk operational action is rejected

- **WHEN** a route, repository or environment operation fails in a view test
- **THEN** the test SHALL verify visible local feedback and retained retry state
