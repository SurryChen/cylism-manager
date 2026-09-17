## MODIFIED Requirements

### Requirement: Observe and configure proxies independently

The platform SHALL list each Registry Proxy's Registry, upstream, endpoint, node, cache configuration, readiness and latest error in the delivery artifact registry workspace. It SHALL allow configuring, diagnosing, migrating legacy resources, and clearing an individual Proxy cache from that workspace.

#### Scenario: View multiple proxies in the delivery center

- **WHEN** a user opens the Registry Proxy workspace in the delivery center
- **THEN** the platform SHALL show Docker Hub and Kubernetes Registry Proxy instances as separate entries
- **AND THEN** it SHALL not imply that one endpoint can proxy both Registries

#### Scenario: Maintain a Proxy without changing node configuration

- **WHEN** a user creates, edits, diagnoses, or clears a Registry Proxy cache
- **THEN** the platform SHALL perform only the requested Proxy lifecycle action
- **AND THEN** it SHALL not create, edit, apply, or restart a node registry mirror configuration
