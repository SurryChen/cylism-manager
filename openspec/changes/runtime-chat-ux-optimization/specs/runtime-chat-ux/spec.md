## ADDED Requirements

### Requirement: Concurrent session streams remain isolated

The chat UI SHALL allow switching and creating sessions while another session is streaming, and SHALL apply each delta, completion, stop, and error event only to the session that started the request.

#### Scenario: Interleaved streams

- **WHEN** sessions A and B stream concurrently and their deltas arrive interleaved
- **THEN** A's deltas appear only in A
- **AND THEN** B's deltas appear only in B

#### Scenario: Stop one stream

- **WHEN** the user stops session A while session B is streaming
- **THEN** only A is marked stopped
- **AND THEN** B continues receiving deltas

### Requirement: Chat progress is explicit

The chat UI SHALL render explicit pending, streaming, stopped, completed, and retryable error states for assistant messages.

#### Scenario: Thinking state

- **WHEN** a request is accepted but no delta has arrived
- **THEN** the assistant bubble displays `正在思考...`
- **AND THEN** the text is replaced by streamed content after the first delta

#### Scenario: Retryable failure

- **WHEN** a request fails before completion
- **THEN** the assistant bubble displays a failure state and a retry action
- **AND THEN** the original session ID remains selected

### Requirement: Completed responses do not reload full history

The UI SHALL refresh session summaries after completion without fetching the active session's full message history again.

#### Scenario: Stream completion

- **WHEN** a response completes successfully
- **THEN** the local assistant message is retained
- **AND THEN** only session summaries are refreshed

### Requirement: Session history is paginated from newest messages

The Runtime session API SHALL return bounded newest-message pages and cursors for loading older messages.

#### Scenario: Initial history page

- **WHEN** a session history request omits a cursor
- **THEN** the Runtime returns the newest bounded page in chronological order
- **AND THEN** it reports whether an older page exists

#### Scenario: Older history page

- **WHEN** the client requests a cursor for older messages
- **THEN** the Runtime returns the preceding bounded page without duplicating the boundary message

### Requirement: Session refresh preserves local new sessions

The UI SHALL keep a locally created session visible until its first request succeeds or the user discards it.

#### Scenario: New session request failure

- **WHEN** the first request of a new session fails
- **THEN** the local session and failed message remain available for retry
- **AND THEN** the failed session is not presented as persisted Runtime history after a full reload
