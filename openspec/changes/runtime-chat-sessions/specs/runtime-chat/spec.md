## ADDED Requirements

### Requirement: The platform proxies runtime chat with SSE streaming

The system SHALL expose an authenticated chat endpoint that relays the runtime's streaming response to the browser as Server-Sent Events. It SHALL propagate client cancellation to the runtime so generation stops.

#### Scenario: Stream a chat reply
- **WHEN** an authenticated user sends a message to a managed runtime chat endpoint
- **THEN** the system returns a Server-Sent Events stream with delta events as the runtime generates the reply
- **AND THEN** it terminates the stream with a done event after the runtime finishes

#### Scenario: Stop generation on client disconnect
- **WHEN** the browser disconnects or aborts the chat stream while the runtime is still generating
- **THEN** the system cancels the upstream runtime request immediately
- **AND THEN** it does not keep generating or hold the connection open

#### Scenario: Reject unauthenticated chat requests
- **WHEN** a request to the chat endpoint has no valid JWT
- **THEN** the system rejects it without contacting the runtime

### Requirement: Session history is read from the runtime

The system SHALL list sessions and read session messages from the runtime through the adapter contract, without persisting conversation state in the Manager.

#### Scenario: List runtime sessions
- **WHEN** an authenticated user opens the chat drawer for a managed runtime
- **THEN** the system returns the runtime's session list
- **AND THEN** the user can select a session to continue

#### Scenario: Load session history
- **WHEN** the user selects a session
- **THEN** the system returns the session messages from the runtime
- **AND THEN** the chat drawer renders the existing history before new messages

#### Scenario: Resume after reopening the drawer
- **WHEN** the user closes and reopens the chat drawer for the same runtime
- **THEN** the system reloads sessions and messages from the runtime
- **AND THEN** the conversation continues without lost context

### Requirement: Runtime credentials never reach the browser

The system SHALL keep the runtime API key inside the Manager and inject it only on upstream runtime requests.

#### Scenario: Browser never receives the runtime key
- **WHEN** the frontend chats with a runtime
- **THEN** the runtime API key appears in no browser-visible response
- **AND THEN** all upstream calls carry the key injected by the Manager

### Requirement: The runtime sidecar exposes read-only session endpoints

The runtime image SHALL provide a session API that lists sessions and returns session messages using the same authentication as the runtime chat API.

#### Scenario: Authorized session read
- **WHEN** the Manager calls the session API with the runtime API key
- **THEN** the sidecar returns the session list and message history from the shared workspace

#### Scenario: Reject missing session credentials
- **WHEN** a request to the session API has no or invalid runtime API key
- **THEN** the sidecar rejects the request without reading session data
