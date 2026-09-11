# frontend-composable-foundations Specification

## Purpose
TBD - created by archiving change frontend-composable-foundations. Update Purpose after archive.
## Requirements
### Requirement: Routed tab state is reusable and query-safe

The frontend SHALL provide a `useRoutedTab` composable that derives the active tab from an allowed tab list and updates only the tab query parameter while preserving other query parameters.

#### Scenario: Invalid or missing tab uses the page default

- **WHEN** the current route has no `tab` query value or the value is not present in the configured tab list
- **THEN** `activeTab` SHALL equal the configured default tab

#### Scenario: Selecting a tab preserves unrelated query parameters

- **WHEN** a page selects a valid tab while the route contains other query parameters
- **THEN** the composable SHALL navigate to the same page with the selected `tab` and preserve the unrelated query parameters

### Requirement: Body scroll locking is lifecycle-safe

The frontend SHALL provide a `useBodyScrollLock` composable that locks document body scrolling while active and restores the prior body overflow value when deactivated or disposed.

#### Scenario: A locked scope prevents body scrolling

- **WHEN** a component activates `useBodyScrollLock`
- **THEN** `document.body.style.overflow` SHALL prevent page scrolling

#### Scenario: Disposing one of multiple locks does not unlock another

- **WHEN** two component scopes activate the scroll lock and one scope is disposed
- **THEN** body scrolling SHALL remain locked until the final active scope is released

### Requirement: Async action state is consistent

The frontend SHALL provide a `useActionState` composable that exposes running, error, and result state for an asynchronous action and always clears the running state after completion.

#### Scenario: A successful action exposes its result

- **WHEN** `run` executes an action that resolves successfully
- **THEN** the composable SHALL expose the resolved result, clear the previous error, and set `running` to false

#### Scenario: A failed action preserves the error and releases running state

- **WHEN** `run` executes an action that rejects
- **THEN** the composable SHALL expose the rejection error, set `running` to false, and allow the caller to handle the failure

#### Scenario: A new action does not retain a previous error

- **WHEN** `run` starts after a previous action failed
- **THEN** the composable SHALL clear the previous error before executing the new action

### Requirement: Existing page behavior remains unchanged after adoption

Pages adopting the composables SHALL preserve existing route URLs, query parameters, API requests, user-visible error handling, and cleanup behavior.

#### Scenario: Existing tab pages keep their route contract

- **WHEN** a user switches tabs in Monitoring, Cluster, Network, Resources, or System Settings
- **THEN** the existing page path and non-tab query parameters SHALL remain unchanged

#### Scenario: Terminal components clean up on unmount

- **WHEN** a ServerTerminal or PodTerminal component is unmounted
- **THEN** its terminal resources SHALL be disposed and body scrolling SHALL be restored when no other lock remains

