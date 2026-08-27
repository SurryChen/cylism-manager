## ADDED Requirements

### Requirement: Delivery center navigation entry

The system SHALL expose a Delivery Center navigation entry for authenticated users. The entry SHALL include a Registry view and SHALL not replace existing node mirror or application release navigation.

#### Scenario: Open the managed Registry view

- **WHEN** an authenticated user selects Delivery Center and then Registry
- **THEN** the system SHALL navigate to the managed Registry view and display its deployment, endpoint, transport, data-node and node-application status
