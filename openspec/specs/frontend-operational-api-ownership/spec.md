# frontend-operational-api-ownership Specification

## Purpose
TBD - created by archiving change frontend-operational-read-consistency. Update Purpose after archive.
## Requirements
### Requirement: Operational endpoint ownership is explicit

Each operational business domain SHALL expose related REST calls from one named API module under `web/src/api/`, and views SHALL not assemble those domain endpoint paths directly. Shared endpoints such as cluster node reads SHALL have one canonical exported owner.

#### Scenario: A view performs an operational read or mutation

- **WHEN** Workloads, Configs, AuditLogs, or NodeRegistryMirrors performs a REST operation
- **THEN** the view SHALL call a named domain function and SHALL pass request options separately from payload data

#### Scenario: A shared endpoint is used by multiple domains

- **WHEN** more than one view needs the same endpoint such as `/nodes`
- **THEN** one API module SHALL own the implementation and other modules SHALL import or re-export that function rather than duplicate its request definition

### Requirement: API modules preserve request contracts

Domain API functions SHALL preserve existing HTTP methods, paths, payload shapes, query parameter names, and URL encoding, and SHALL accept optional request options for cancellable reads.

#### Scenario: An audit query is encoded

- **WHEN** a caller supplies page, offset, resource type, action, keyword, sort, or order
- **THEN** the audit API function SHALL encode only supplied values using the existing parameter names and return the unwrapped API data

#### Scenario: A Kubernetes resource name contains reserved characters

- **WHEN** a caller requests a namespaced workload or configuration detail
- **THEN** the API function SHALL URL-encode namespace and name segments exactly once

### Requirement: Stable ChatDrawer regions remain independently testable

If ChatDrawer regions are extracted, child components SHALL receive explicit props and emit user actions, while stream transport, retry, session selection orchestration, and approval side effects remain owned by the parent.

#### Scenario: A session or approval list is extracted

- **WHEN** the parent renders an extracted list region
- **THEN** the child SHALL render from props and emit an action event without importing parent-local stream state

#### Scenario: A user performs an existing ChatDrawer action

- **WHEN** the user selects, archives, retries, approves, or rejects an item
- **THEN** the same parent handler, endpoint, payload, loading state, and visible result SHALL be used

