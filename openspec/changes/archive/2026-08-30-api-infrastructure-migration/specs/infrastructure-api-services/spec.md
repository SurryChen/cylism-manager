## ADDED Requirements

### Requirement: Infrastructure HTTP adapters preserve external contracts

The system SHALL preserve the existing REST paths, authorization behavior, response wrapper, error code and response fields when an infrastructure Handler is migrated into `internal/api/infrastructure`.

#### Scenario: Existing server request after migration

- **WHEN** an authorized client calls an existing server endpoint with the same request as before migration
- **THEN** the endpoint SHALL retain its HTTP method, path, response wrapper and semantic result

### Requirement: Infrastructure business workflows are reusable without Gin

The system SHALL place server/node, storage, and network business workflows in domain Services that do not depend on Gin context or API packages.

#### Scenario: Service workflow test

- **WHEN** a domain workflow is invoked by a unit test with fake infrastructure dependencies
- **THEN** it SHALL execute or return a classified business error without requiring an HTTP request

### Requirement: Kubernetes resource adapters remain transport-independent

The system SHALL keep reusable Kubernetes resource operations outside HTTP Handlers and SHALL inject SSH or Agent side effects through explicit interfaces.

#### Scenario: Handler executes a side-effecting infrastructure operation

- **WHEN** an authorized HTTP request triggers a server, node, storage, or network operation
- **THEN** the Handler SHALL map the request and response while the Service invokes the injected infrastructure adapter

### Requirement: Migration is independently verifiable by domain

The system SHALL migrate one infrastructure domain at a time and SHALL retain focused regression tests before beginning the next domain.

#### Scenario: Completed domain migration

- **WHEN** a server/node, storage, or network migration task is marked complete
- **THEN** focused tests for that domain and the full backend test suite SHALL pass before another domain is migrated
