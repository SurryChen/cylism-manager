# frontend-page-state-hardening Specification

## Purpose
Improve frontend request, authentication, component, test, and bundle behavior so operational pages remain correct during rapid navigation, concurrent session expiry, failures, and repeated maintenance workflows.

## ADDED Requirements

### Requirement: Managed page reads use cancellable domain resources

Data-heavy views SHALL use named functions from `web/src/api/` and `useAsyncResource` for reads that can overlap due to route, filter, tab, or pagination changes.

#### Scenario: A view changes context before its previous read completes

- **WHEN** the user changes the view context while a managed read is pending
- **THEN** the view SHALL cancel the previous request, start the new domain API read with an `AbortSignal`, and SHALL NOT render the older response

#### Scenario: A managed read fails after a successful read

- **WHEN** a later refresh fails after the view has loaded data successfully
- **THEN** the view SHALL retain the last successful data and expose the failure in the affected region

#### Scenario: A managed view is unmounted during a read

- **WHEN** the component is unmounted before its managed request resolves
- **THEN** the request SHALL be cancelled when possible and SHALL NOT update disposed component state

### Requirement: Token refresh is single-flight

The API request layer SHALL coordinate concurrent access-token refreshes so that at most one refresh request is in flight for a given expired session.

#### Scenario: Several requests expire at the same time

- **WHEN** multiple API requests receive HTTP 401 while a refresh token is available
- **THEN** they SHALL await one refresh request, retry with the resulting access token, and SHALL NOT overwrite each other with competing refresh results

#### Scenario: Refresh fails

- **WHEN** the refresh request fails or returns an invalid response
- **THEN** the API layer SHALL clear stored tokens, perform one login redirect, and return an authentication error to the original callers

#### Scenario: A request is cancelled

- **WHEN** a caller aborts a managed request during authentication handling
- **THEN** cancellation SHALL remain distinguishable from authentication failure and SHALL NOT be rendered as a page error

### Requirement: Large views preserve orchestration boundaries

Oversized views SHALL be split only at coherent, independently testable UI boundaries, while page-level route state, domain orchestration, and mutation contracts remain unchanged.

#### Scenario: A region is extracted from a large view

- **WHEN** a server, monitoring, system-component, or workload region is extracted
- **THEN** the child SHALL receive explicit props, emit user actions, and SHALL NOT access unrelated parent-local state through imports

#### Scenario: Existing page behavior is rendered through a child

- **WHEN** the user performs an existing action in an extracted region
- **THEN** the same endpoint, payload, confirmation, loading state, and visible result SHALL be preserved

### Requirement: High-risk frontend workflows have regression coverage

The frontend SHALL provide focused tests for authentication transitions, data loading failures, stale-result protection, unmount behavior, important mutations, and named API functions in operational pages.

#### Scenario: A high-risk read fails

- **WHEN** a certificate, cluster, resource, settings, or database read rejects
- **THEN** its test SHALL verify a visible local error state and that unrelated data regions remain intact where applicable

#### Scenario: A high-risk mutation fails

- **WHEN** a login, certificate, cluster, settings, or database mutation rejects
- **THEN** its test SHALL verify the submitting state is released and the failure is surfaced without falsely reporting success

#### Scenario: A page is disposed during an asynchronous workflow

- **WHEN** a tested page is unmounted while a read or refresh is pending
- **THEN** its test SHALL verify no later response changes the disposed page state

### Requirement: Optional frontend capabilities do not inflate initial route payloads

Terminal and chart capabilities SHALL be loaded only when needed when dynamic loading is compatible with the current route behavior, and confirmed empty or obsolete frontend artifacts SHALL not remain in `web/src`.

#### Scenario: A user opens an optional terminal or chart region

- **WHEN** the user requests the optional capability
- **THEN** the capability SHALL load before use, show the existing loading or failure state if loading fails, and preserve the current interaction

#### Scenario: An obsolete frontend artifact is reviewed

- **WHEN** a file has no imports, route references, build references, or test references
- **THEN** it MAY be removed after verification, without deleting files that are still part of a supported workflow
