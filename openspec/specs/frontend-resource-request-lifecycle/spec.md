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

The applications, servers, and monitoring views SHALL use named domain API functions for managed read operations, and those functions SHALL accept an optional request options object containing an AbortSignal.

#### Scenario: A managed resource starts a domain read

- **WHEN** a view refreshes a managed application, server, or monitoring resource
- **THEN** it SHALL invoke the corresponding domain API function with the resource AbortSignal

#### Scenario: Domain read query parameters are supplied

- **WHEN** a domain read needs a project, environment, server, tab, or time range parameter
- **THEN** the domain API function SHALL encode that parameter in the existing REST request without changing the backend contract

### Requirement: Read failures remain local to their page region

The frontend SHALL expose managed read failures through the resource error state without clearing the last successful data for an unrelated page region.

#### Scenario: Monitoring trends fail while status is available

- **WHEN** a monitoring trend request fails after the monitoring status request has succeeded
- **THEN** the status area SHALL remain available and the trend area SHALL retain its local error/loading outcome

#### Scenario: A later refresh fails after a successful read

- **WHEN** a managed resource has data and a later refresh fails
- **THEN** the resource SHALL retain the previous data and expose the new error state

