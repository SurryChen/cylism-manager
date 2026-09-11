# frontend-observability-api-boundary Specification

## Purpose
TBD - created by archiving change frontend-observability-api-boundary. Update Purpose after archive.
## Requirements
### Requirement: Observability endpoint ownership is explicit

Certificate, monitoring, and managed OCI Registry pages SHALL call named functions from their domain API modules for reads and mutations, rather than assembling domain endpoint paths in the views.

#### Scenario: An administrator changes observability configuration

- **WHEN** a user installs, configures, tests, migrates, repairs, or removes a certificate, monitoring component, or managed Registry
- **THEN** the page SHALL call a named domain API function with request options separate from the business payload

### Requirement: Existing observability HTTP contracts are preserved

Domain API functions SHALL preserve existing HTTP methods, paths, query parameter names, URL encoding, payload shapes, and unwrapped return values.

#### Scenario: A monitoring query includes a time range

- **WHEN** a user selects a supported range or query string
- **THEN** the request SHALL encode the same range/query parameters and use the same monitoring endpoint as before the migration

### Requirement: Current observability reads win races

Route-, tab-, range-, and form-dependent reads SHALL cancel superseded requests or ignore their results, and SHALL commit only the current request result.

#### Scenario: A user changes the monitoring range quickly

- **WHEN** a second trend query starts before the first query completes
- **THEN** the first result SHALL NOT overwrite the second result or its loading state

#### Scenario: A certificate or Registry page unmounts during a read

- **WHEN** the owning page is unmounted while a read is pending
- **THEN** the request SHALL be cancelled or ignored and SHALL NOT update component state or display an abort error

### Requirement: Polling follows page and workflow lifecycle

Status polling SHALL not overlap requests, SHALL stop on unmount, and SHALL stop or change interval when the monitored workflow reaches its terminal state.

#### Scenario: A monitoring migration completes

- **WHEN** a polled migration reaches a terminal success or failure state
- **THEN** the page SHALL stop or reduce polling according to the existing workflow behavior and SHALL retain the final status

### Requirement: Observability errors are scoped and non-destructive

A failed observability read or mutation SHALL show an error in the smallest applicable region and SHALL preserve unrelated successful data.

#### Scenario: Registry certificate options fail while registry data is loaded

- **WHEN** the certificate-options request fails after the Registry status succeeds
- **THEN** the Registry status and existing form data SHALL remain available and only the certificate-options region SHALL show the failure

