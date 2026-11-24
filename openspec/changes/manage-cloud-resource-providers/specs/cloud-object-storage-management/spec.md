## ADDED Requirements

### Requirement: Object-storage containers are provider-neutral resources

The system SHALL list, create, update supported configuration for, and delete containers accessible through an enabled object-storage capability. Common container data SHALL include name and region where available; provider-specific configuration SHALL be returned only as safe optional metadata.

#### Scenario: Delete a non-empty container
- **WHEN** a user requests deletion of a container that contains objects
- **THEN** the system rejects the request and does not delete the container or its objects

### Requirement: Objects are managed through bounded server streams

The system SHALL list objects by container and optional prefix with provider pagination, and SHALL support authenticated streaming upload, download, and explicit single-object deletion. Operations MUST remain scoped to the selected connection and container.

#### Scenario: Upload an object
- **WHEN** a user uploads a file within the configured size limit to a selected container and object key
- **THEN** the system streams the file through the selected provider adapter, returns safe object metadata, and audits the outcome

#### Scenario: Delete an object without confirmation
- **WHEN** a caller omits explicit confirmation for an object deletion
- **THEN** the system rejects the request and does not invoke the provider deletion API
