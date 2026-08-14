## MODIFIED Requirements

### Requirement: Agent write operations require immutable human approval

The system SHALL require human approval for every first-release Agent mutation and SHALL execute approved mutations only from the Manager control plane. Each operation SHALL retain an opaque originating chat-session reference when the Runtime provided a valid reference; that reference SHALL be informational only and SHALL NOT affect authorization, parameters, expiry, or execution.

#### Scenario: Terminal approval resolution refreshes the queue

- **WHEN** an administrator approves or rejects a pending operation and the Manager transitions it to succeeded, failed, stale, expired, or rejected
- **THEN** the pending approval view SHALL remove it after reloading authoritative pending data
- **AND THEN** the UI SHALL display the terminal status or bounded failure reason

#### Scenario: Approval history is inspected

- **WHEN** an administrator opens Agent operation history for a Runtime or chat session
- **THEN** the system SHALL return bounded, cursor-paginated immutable operation records with status, summary, relevant timestamps, and bounded result summary
- **AND THEN** raw parameters and credentials SHALL NOT be returned for display

#### Scenario: Chat session correlation is present

- **WHEN** a Runtime starts an approved operation from a valid active chat session
- **THEN** the Manager SHALL retain that session ID on the immutable operation record
- **AND THEN** the chat UI SHALL use the reference only to display the matching operation state in that conversation
