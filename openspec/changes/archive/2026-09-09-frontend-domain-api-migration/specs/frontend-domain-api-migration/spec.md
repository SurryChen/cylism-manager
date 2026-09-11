# Frontend Domain API Migration

## ADDED Requirements

### Requirement: Domain modules own managed read requests
The frontend SHALL expose named read functions for each migrated domain instead of constructing those read URLs inline in the page.

#### Scenario: A page loads managed data
- **WHEN** a migrated page loads or refreshes data
- **THEN** it calls the corresponding domain API function
- **AND** the function uses the existing endpoint and response contract

### Requirement: Managed reads support cancellation
Each migrated read function SHALL accept request options and forward `signal` to the generic API client.

#### Scenario: A route or filter changes during a request
- **WHEN** a new managed read starts before the previous one completes
- **THEN** the previous request is aborted or treated as stale
- **AND** its result does not overwrite the current page state

### Requirement: Mutations preserve existing behavior
The migration SHALL NOT change mutation endpoints or payloads.

#### Scenario: A user creates, updates, or deletes a resource
- **WHEN** the mutation succeeds
- **THEN** the page refreshes through the domain read function
- **AND** the existing success and error behavior remains intact

### Requirement: Polling follows component lifecycle
Migrated workspace polling SHALL stop when the component is unmounted or its resource context changes.

#### Scenario: A user leaves a polling page
- **WHEN** the page is unmounted
- **THEN** no later poll result updates the unmounted page
