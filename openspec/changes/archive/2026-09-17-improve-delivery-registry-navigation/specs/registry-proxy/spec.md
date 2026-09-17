## MODIFIED Requirements

### Requirement: Observe and configure proxies independently

The platform SHALL list each Registry Proxy's Registry, upstream, endpoint, node, cache configuration, readiness and latest error in the delivery artifact registry workspace. It SHALL allow configuring, diagnosing, migrating legacy resources, and clearing an individual Proxy cache from that workspace. The Proxy workspace SHALL be canonically reachable at `/delivery/registry?tab=registry-proxy`, and each Proxy instance SHALL use the same glass card surface as the managed Registry status metrics while preserving its internal property-grid separators. The workspace SHALL not repeat its top-level `Registry Proxy` Tab label as an inner page heading.

#### Scenario: View multiple proxies in the delivery center

- **WHEN** a user opens `/delivery/registry?tab=registry-proxy`
- **THEN** the platform SHALL show Docker Hub and Kubernetes Registry Proxy instances as separate glass-surface cards
- **AND THEN** each card SHALL retain visible internal separators between its operational properties
- **AND THEN** it SHALL not imply that one endpoint can proxy both Registries

#### Scenario: View an upstream connectivity result

- **WHEN** a Proxy has a saved upstream diagnostic result
- **THEN** the platform SHALL display its status and safe diagnostic summary together in an `上游连通性` property-grid cell
- **AND THEN** a `healthy` result, including “代理 Pod 可访问上游 Registry”, SHALL use the success presentation rather than an error presentation
- **AND THEN** the platform SHALL reserve standalone error text for an actual Proxy lifecycle error

#### Scenario: Maintain a Proxy without changing node configuration

- **WHEN** a user creates, edits, diagnoses, or clears a Registry Proxy cache
- **THEN** the platform SHALL perform only the requested Proxy lifecycle action
- **AND THEN** it SHALL not create, edit, apply, or restart a node registry mirror configuration
