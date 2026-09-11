# frontend-polling-lifecycle Specification

## Purpose
TBD - created by archiving change frontend-polling-lifecycle. Update Purpose after archive.
## Requirements
### Requirement: Polling has an explicit lifecycle
The frontend SHALL provide a reusable polling composable with explicit start and stop operations.

#### Scenario: A page starts polling
- **WHEN** a page starts polling a pending operation
- **THEN** the callback runs immediately when requested
- **AND** subsequent callbacks run at the configured interval

#### Scenario: A page stops polling
- **WHEN** the page reaches a terminal state or is unmounted
- **THEN** the timer is cleared
- **AND** no later callback is scheduled

### Requirement: Polling prevents overlapping callbacks
The polling composable SHALL wait for an asynchronous callback to settle before invoking it again.

#### Scenario: A request exceeds the polling interval
- **WHEN** a poll callback is still pending at the next interval
- **THEN** the next invocation is skipped
- **AND** a second request is not started concurrently

### Requirement: Polling errors remain page-owned
The polling composable SHALL allow callback failures to settle without creating unhandled promise rejections or disabling future polls.

#### Scenario: A poll request fails transiently
- **WHEN** the callback rejects
- **THEN** the page can update its existing error state
- **AND** later polling remains possible until the page stops it

### Requirement: Polling is scope-safe
Polling SHALL stop automatically when the Vue effect scope owning it is disposed.

#### Scenario: A user navigates away from a polling page
- **WHEN** the component scope is disposed
- **THEN** its timer is cleared
- **AND** no callback updates the unmounted page

