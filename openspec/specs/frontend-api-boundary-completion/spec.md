# frontend-api-boundary-completion Specification

## Purpose
TBD - created by archiving change frontend-api-boundary-completion. Update Purpose after archive.
## Requirements
### Requirement: Remaining frontend workflows use named domain API functions

Login, Runtime management, Runtime chat, database administration, and certificate-operation views SHALL invoke named functions from their owning frontend API module rather than constructing operational REST paths in the view or component. The generic API module SHALL retain only transport, response-unwrapping, token, and token-refresh responsibilities.

#### Scenario: An operator performs a Runtime or database action

- **WHEN** an operator saves, deploys, checks, uninstalls, authorizes, resolves, creates, updates, or deletes from the Runtime or database administration UI
- **THEN** the initiating view SHALL call a named function from `runtimes.js` or `admin.js` that preserves the existing HTTP method, encoded path, query, payload, and unwrapped response

#### Scenario: A user authenticates or uses Runtime chat

- **WHEN** a user signs in with credentials or a temporary token, or uses a Runtime chat session, stream, or approval action
- **THEN** the UI SHALL call a named function from `auth.js` or `runtimes.js` and SHALL retain the existing authentication and streaming protocol behavior

#### Scenario: A feature-specific endpoint is moved from the transport module

- **WHEN** a Runtime chat or Agent endpoint is owned by `runtimes.js`
- **THEN** `api/index.js` SHALL NOT export that endpoint-specific function and existing production consumers SHALL import it from `runtimes.js`

### Requirement: Certificate-operation reads are current and cancellable

The certificate-operation detail view SHALL load operations through a request lifecycle tied to the active namespace and certificate name, and SHALL not commit a superseded or disposed request result.

#### Scenario: An operator switches certificate detail routes during a pending request

- **WHEN** the namespace or certificate name changes before an earlier operation request settles
- **THEN** the earlier response SHALL NOT replace the active route's operations, loading state, or error state

#### Scenario: The certificate-operation view unmounts during a pending request

- **WHEN** the certificate-operation view is removed while its request is pending
- **THEN** the request SHALL be cancelled or ignored and the page SHALL NOT surface an abort error

### Requirement: Rejected operational mutations retain retryable local state

Runtime and database administration mutation failures SHALL remain visible in their initiating page region and SHALL preserve the relevant retryable form, confirmation target, and already loaded data.

#### Scenario: A Runtime lifecycle or authorization action is rejected

- **WHEN** saving a Runtime, running a lifecycle action, saving Agent grants, or resolving an Agent operation is rejected
- **THEN** Runtime Management SHALL display local failure feedback and SHALL retain the active form, target, or approval state needed to retry

#### Scenario: A database record mutation is rejected

- **WHEN** creating, editing, or deleting a database administration record is rejected
- **THEN** DB Admin SHALL show local failure feedback and SHALL retain the relevant editor values or deletion target without clearing the loaded table rows

### Requirement: API-boundary completion has focused regression coverage

The frontend SHALL provide contract and view tests for the moved operational API ownership, certificate request lifecycle, and rejected mutation retry behavior.

#### Scenario: A future refactor changes a named operational request contract

- **WHEN** a named authentication, Runtime, database, or certificate function changes its method, path, identifier encoding, payload, request options, or stream behavior
- **THEN** focused API module tests SHALL detect the regression

#### Scenario: A high-risk interaction fails or becomes stale

- **WHEN** a Runtime or database mutation is rejected, or a certificate request is superseded or cancelled
- **THEN** focused view tests SHALL verify visible local feedback, retained retry state, and isolation of stale or disposed results

