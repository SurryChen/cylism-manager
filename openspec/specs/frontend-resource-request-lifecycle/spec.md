# frontend-resource-request-lifecycle Specification

## Purpose
TBD - created by archiving change frontend-resource-request-lifecycle. Update Purpose after archive.
## Requirements
### Requirement: Latest page read result is authoritative

The frontend SHALL cancel an in-flight managed read when the same resource is refreshed, and SHALL update page state only from the most recently started managed read.

#### Scenario: A filter changes before its previous request completes

- **WHEN** a user changes an application workspace, server section, monitoring tab, or monitoring time range before its current managed read completes
- **THEN** the frontend SHALL start a new read and SHALL not render the prior read's result after the newer read has started

#### Scenario: A page is left with a managed read in progress

- **WHEN** a component using a managed resource is unmounted before its request completes
- **THEN** the frontend SHALL abort the request and SHALL not update that component's state from its response

### Requirement: High-frequency reads use domain API functions

The applications, servers, monitoring, workloads, configuration, audit, and node registry mirror views SHALL use named domain API functions for managed reads and SHALL pass an optional request options object containing an `AbortSignal`.

#### Scenario: A managed operational resource starts a domain read

- **WHEN** a target view refreshes a workload, configuration tab, audit page, mirror list, proxy list, or apply status
- **THEN** it SHALL invoke the corresponding domain API function with the resource AbortSignal

#### Scenario: Domain read query parameters are supplied

- **WHEN** a domain read needs a namespace, resource name, audit filter, page, sort, or mirror identifier
- **THEN** the domain API function SHALL encode that parameter in the existing REST request without changing the backend contract

### Requirement: Read failures remain local to their page region

The frontend SHALL expose managed read failures through purpose-specific resource error state (`listError`, `detailError`, or `pollingError`) without clearing the last successful data for an unrelated page region. An aborted request SHALL NOT be rendered as a user-facing failure.

#### Scenario: A detail request fails while the list is available

- **WHEN** a workload, ConfigMap, or Secret detail request fails after its list request has succeeded
- **THEN** the list SHALL remain available and only the detail region SHALL expose its error

#### Scenario: A later refresh fails after a successful read

- **WHEN** a managed resource has data and a later refresh fails
- **THEN** the resource SHALL retain the previous data and expose the new error state for that resource

#### Scenario: An obsolete request is aborted

- **WHEN** a filter, tab, pagination, route, or component disposal aborts a pending request
- **THEN** the aborted request SHALL not replace newer data and SHALL not set a visible page error

### Requirement: Operational mutations expose local outcome state

Workloads, Configs, and NodeRegistryMirrors SHALL expose submitting state while a mutation is pending, release it on both success and failure, show a local mutation error on failure, and refresh only the affected resource after success.

#### Scenario: A workload mutation fails

- **WHEN** scale, image update, or rollback is rejected
- **THEN** the dialog SHALL remain usable, submitting state SHALL be cleared, and a visible mutation error SHALL be shown without falsely reporting success

#### Scenario: A configuration mutation succeeds

- **WHEN** a ConfigMap or Secret is created, edited, or deleted successfully
- **THEN** the editor state SHALL close or update as before and only the active configuration list SHALL refresh

### Requirement: Apply progress polling follows component lifecycle

NodeRegistryMirrors SHALL start apply-status polling only for active applications, stop when all applications finish or the view unmounts, and keep polling failures separate from list and mutation errors.

#### Scenario: An apply operation completes

- **WHEN** every active mirror reports a terminal apply status
- **THEN** polling SHALL stop and the corresponding mirror rows SHALL show their final statuses

#### Scenario: The mirror page is unmounted while polling

- **WHEN** the view is disposed while apply-status polling is active
- **THEN** the timer and in-flight reads SHALL be cancelled and no disposed state SHALL be updated

#### Scenario: Polling fails

- **WHEN** an apply-status read rejects for a transient or server error
- **THEN** the page SHALL expose a polling-specific error and SHALL preserve the last known mirror and proxy data

