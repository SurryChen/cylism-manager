## ADDED Requirements

### Requirement: Chat sessions use Nanobot-native lifecycle management

The system SHALL allow an authenticated operator to rename, archive, restore, export, or permanently delete an active Runtime chat session through the Manager proxy. These actions SHALL be scoped to the Runtime-owned `api:` session namespace and SHALL use Nanobot's native session or sidebar-state helpers.

#### Scenario: Rename a session

- **WHEN** an operator supplies a non-empty bounded title for an existing session
- **THEN** the session list SHALL display the updated Nanobot sidebar title override after refresh
- **AND THEN** the session's messages and durable memory SHALL remain unchanged

#### Scenario: Archive a session

- **WHEN** an operator archives an inactive session
- **THEN** the Runtime SHALL add its key to Nanobot's persisted archived session metadata and remove it from the active session listing
- **AND THEN** its session file, durable memory, workspace files, and Runtime PVC SHALL remain unchanged

#### Scenario: Permanently delete a session

- **WHEN** an operator confirms permanent deletion of an inactive session
- **THEN** the Runtime SHALL delete only the matching Nanobot session through `SessionManager.delete_session`
- **AND THEN** the operator SHALL be able to export the native session snapshot before deletion
- **AND THEN** the UI SHALL disclose that prior Nanobot compaction may already have moved older context to `memory/history.jsonl`

#### Scenario: Delete an active stream is prevented

- **WHEN** an operator attempts to delete a session with an active chat stream
- **THEN** the UI SHALL require that stream to be stopped before deletion
- **AND THEN** streams in other sessions SHALL continue normally

### Requirement: Chat displays related approval lifecycle

The chat UI SHALL present the status and bounded result of Agent operations that are correlated to the selected session, while preserving a Runtime-wide pending approval entry point.

#### Scenario: Session operation result

- **WHEN** an approved or rejected operation has a matching chat session reference
- **THEN** the selected conversation SHALL show its current status and bounded result summary
- **AND THEN** the displayed result SHALL not imply that the original model stream is still active
