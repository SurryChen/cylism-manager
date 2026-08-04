## ADDED Requirements

### Requirement: An application supports multiple managed endpoints

The system SHALL allow an application to maintain multiple public endpoints. Each endpoint SHALL reference one enabled managed domain in the application's environment, one normalized path, and its derived TLS configuration.

#### Scenario: Bind a second domain
- **WHEN** an administrator binds a second ready managed domain to an application that already has a public endpoint
- **THEN** the system retains the existing endpoint and adds the new endpoint to the application's endpoint list
- **AND THEN** the application exposes both Hosts through its managed Ingress

#### Scenario: Preserve a legacy single endpoint
- **WHEN** an application has an endpoint created before multi-endpoint support
- **THEN** the endpoint is returned as one item in the endpoint list without data migration or loss

### Requirement: Endpoint routes are isolated and conflict-safe

The system SHALL reject a duplicate managed domain and path route that is already assigned to a different application or already present on the same application. It SHALL allow distinct paths on the same Host when no conflicting endpoint exists.

#### Scenario: Reject a duplicate Host and path
- **WHEN** an administrator binds a domain and path already used by another application's endpoint in the same namespace
- **THEN** the system rejects the request with an actionable route conflict
- **AND THEN** it does not modify the managed Ingress

#### Scenario: Edit one endpoint without affecting another
- **WHEN** an administrator edits the path or TLS setting of one endpoint on an application with multiple endpoints
- **THEN** the system updates only the selected endpoint
- **AND THEN** all other endpoint records and Ingress rules remain present

### Requirement: A managed Ingress reflects all application endpoints

The system SHALL reconcile one platform-managed Ingress per application from the application's complete endpoint list. It SHALL render one Host rule per endpoint and one or more TLS entries derived from the referenced managed domain TLS Secrets.

#### Scenario: Bind endpoints using separate certificates
- **WHEN** an application has two HTTPS endpoints whose managed domains use different TLS Secrets
- **THEN** the generated Ingress contains routing rules for both Hosts and TLS entries for both Secrets

#### Scenario: Remove one of several endpoints
- **WHEN** an administrator removes one endpoint while other endpoints remain
- **THEN** the system updates the managed Ingress to retain the remaining rules
- **AND THEN** it does not delete the Ingress

#### Scenario: Remove the final endpoint
- **WHEN** an administrator removes the final endpoint of an application
- **THEN** the system removes the application's platform-managed Ingress

### Requirement: Releases preserve current endpoint bindings

The system SHALL treat endpoint bindings as application-level state. Creating, retrying, or rolling back a release SHALL synchronize the current endpoint list and SHALL NOT restore a single endpoint from an historical release snapshot.

#### Scenario: Publish after adding a second domain
- **WHEN** an administrator binds a second domain and then creates a new application release
- **THEN** the resulting Ingress continues to expose both domains
