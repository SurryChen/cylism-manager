## ADDED Requirements

### Requirement: Cloud DNS resources are discoverable through provider adapters

The system SHALL list DNS zones and paged records accessible through an enabled DNS capability, including provider resource identifier, host record, type, value, TTL, enabled state, and safe optional metadata.

#### Scenario: List DNS records
- **WHEN** a user opens a DNS zone for a validated DNS-enabled cloud connection
- **THEN** the system returns current provider records and pagination information when more records exist

### Requirement: DNS records can be changed safely

The system SHALL validate common DNS fields, use provider resource identifiers for edits/deletes, pass supported provider options only to the selected adapter, refresh results after mutation where the provider supports it, and audit outcomes.

#### Scenario: Edit a record
- **WHEN** a user edits an existing record using its current provider identifier
- **THEN** the system applies permitted fields through the connection provider and records the update audit event

#### Scenario: Delete without confirmation
- **WHEN** a caller requests DNS record deletion without explicit confirmation
- **THEN** the system rejects the request and does not invoke the provider deletion API
