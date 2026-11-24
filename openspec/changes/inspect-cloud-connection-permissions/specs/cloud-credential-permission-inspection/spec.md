## ADDED Requirements

### Requirement: Inspect only the connection's caller identity

The system SHALL inspect an Alibaba Cloud connection using its encrypted server-side credentials and SHALL return safe caller identity metadata without returning credential plaintext or ciphertext.

#### Scenario: Inspect a RAM user AccessKey
- **WHEN** an authenticated operator requests permission inspection for an Alibaba Cloud connection whose AccessKey belongs to a RAM user
- **THEN** the system returns the caller identity and inspection timestamp without exposing the AccessKey ID or Secret

#### Scenario: Unsupported provider
- **WHEN** an operator requests permission inspection for a provider without an inspection adapter
- **THEN** the system returns an explicit unsupported result without altering the connection or credentials

### Requirement: Report assigned RAM policies with provenance

The system SHALL report policies attached directly to the caller RAM user and policies inherited through that user's groups, including each policy's name, type, and assignment source. The system MUST NOT label assigned policies as effective permissions.

#### Scenario: Direct and inherited assignments
- **WHEN** RAM permits the inspection APIs and the caller has direct policies and group-inherited policies
- **THEN** the result includes both sets with their respective direct or group source and reports a complete inspection

#### Scenario: No attached policies
- **WHEN** all RAM policy and group queries succeed and return no assignments
- **THEN** the result reports a complete inspection with an empty policy list

#### Scenario: Incomplete RAM queries
- **WHEN** one or more RAM queries fail after some identity or policy data has been retrieved
- **THEN** the result preserves safe retrieved data, reports partial or unavailable inspection, and supplies a redacted reason instead of claiming no policies exist

#### Scenario: Non-RAM principal
- **WHEN** the AccessKey caller is not a RAM user that the inspector can enumerate
- **THEN** the result shows the known identity and marks policy inspection unavailable rather than returning a complete empty list

### Requirement: Display inspection separately from connection validation

System Settings SHALL offer an on-demand permission view for supported connections and SHALL distinguish assigned-policy information from DNS/OSS operational validation.

#### Scenario: Open and retry permission view
- **WHEN** an operator opens a supported cloud connection's permission view or retries a failed inspection
- **THEN** the UI requests current inspection data, shows a loading state, and then shows identity, policies, provenance, timestamp, and inspection status

#### Scenario: Missing RAM read grant
- **WHEN** the inspection API reports that RAM policy metadata could not be read
- **THEN** the UI explains that permission inspection is unavailable, keeps existing DNS/OSS connection validation visible, and does not imply that DNS/OSS access failed

#### Scenario: Authorization semantics
- **WHEN** any policy assignments are displayed
- **THEN** the UI states that attached policies are not proof of effective access to a specific resource or operation
