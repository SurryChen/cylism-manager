## ADDED Requirements

### Requirement: Deferred configuration form dependencies

The configuration resource page SHALL defer namespace-option reads until the user opens the create-resource flow, rather than requesting namespaces while the list first loads.

#### Scenario: Browse configuration resources

- **WHEN** a user opens the configuration resource page without opening the create form
- **THEN** the page SHALL load only the active lightweight configuration list
- **AND THEN** the page SHALL NOT request namespace options

#### Scenario: Create a configuration resource

- **WHEN** a user opens the ConfigMap or Secret create form
- **THEN** the page SHALL request namespace options and expose a loading state until they are available
- **AND THEN** the page SHALL retain a namespace text-input fallback if the options cannot be loaded
