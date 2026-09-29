## ADDED Requirements

### Requirement: Cloud connections use provider-neutral secure contracts

The system SHALL allow an authenticated user to create, view, update, validate, and delete a cloud resource connection with a display name, provider key, encrypted provider credential payload, safe provider configuration, and independently enabled supported capabilities. API responses, logs, audit details, and validation errors MUST NOT expose credential plaintext or ciphertext.

#### Scenario: Create a connection from provider schema
- **WHEN** a user selects a supported provider and submits values that satisfy its credential/configuration schema with at least one supported capability enabled
- **THEN** the system encrypts and persists credentials, returns only safe connection metadata and a credential-configured indicator, and records a creation audit event

#### Scenario: Read a connection
- **WHEN** a user requests an existing cloud connection
- **THEN** the response includes provider identity, safe metadata, enabled capabilities, and validation state but excludes credentials

#### Scenario: Reject unsupported capability
- **WHEN** a user enables a capability that the chosen provider does not advertise
- **THEN** the system rejects the request without changing the stored connection

### Requirement: Provider catalog drives connection setup

The system SHALL expose supported provider keys, display metadata, credential/configuration schemas, and available capabilities to authenticated clients without exposing secrets or private adapter details.

#### Scenario: Discover supported providers
- **WHEN** a user opens Cloud Services resource access
- **THEN** the system returns the configured provider catalog and the UI can present a provider selection control

### Requirement: Connection validation diagnoses enabled capabilities

The system SHALL validate each enabled capability through the selected provider adapter and persist timestamped, redacted success or failure state per capability.

#### Scenario: Capability validation fails
- **WHEN** a provider credential cannot access one enabled capability
- **THEN** the system preserves the connection, marks that capability unavailable with a redacted actionable reason, and does not mark it ready
