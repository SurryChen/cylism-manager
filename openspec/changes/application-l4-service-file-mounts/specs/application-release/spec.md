## MODIFIED Requirements

### Requirement: Application templates render Kubernetes Services

The platform SHALL render each application template as a Kubernetes Service with an explicitly validated transport protocol and Service type. Existing templates that do not specify the new fields SHALL retain TCP and ClusterIP behavior.

#### Scenario: Render a legacy or default template

- **WHEN** a template omits Service protocol and type
- **THEN** the platform SHALL render a TCP ClusterIP Service
- **AND THEN** existing HTTP endpoint behavior SHALL remain unchanged

#### Scenario: Render a multi-port LoadBalancer Service

- **WHEN** a template specifies TCP and UDP Service ports with `type=LoadBalancer`
- **THEN** each container port and Service port SHALL use its declared protocol
- **AND THEN** the Service SHALL use type LoadBalancer
- **AND THEN** the platform SHALL expose the allocated Service address and each port as a four-layer endpoint

#### Scenario: Preserve legacy single-port templates

- **WHEN** a template has no Service port list
- **THEN** the platform SHALL derive one Service port from its legacy Service fields
- **AND THEN** the rendered Service SHALL remain compatible with the historical template

#### Scenario: Render a NodePort Service

- **WHEN** a template specifies `type=NodePort` with a valid node port
- **THEN** the Service SHALL contain that node port
- **AND THEN** the configured external traffic policy SHALL be applied when valid

#### Scenario: Reject incompatible Service fields

- **WHEN** a ClusterIP template specifies a node port or external traffic policy
- **THEN** the platform SHALL reject the template before creating Kubernetes resources

### Requirement: Application public endpoints have explicit routing semantics

The platform SHALL distinguish HTTP Ingress routing from direct Service exposure.

#### Scenario: Render HTTP Ingress

- **WHEN** a TCP application endpoint selects HTTP Ingress routing
- **THEN** the platform SHALL render the managed HTTP Ingress using its domain, path and optional TLS Secret

#### Scenario: Render direct Service exposure

- **WHEN** an application endpoint selects Service exposure
- **THEN** the platform SHALL not create an HTTP Ingress
- **AND THEN** the application SHALL be reached through the Service external address and port

#### Scenario: Reject UDP HTTP Ingress

- **WHEN** a UDP Service is paired with HTTP Ingress routing
- **THEN** the platform SHALL reject the configuration before publication

#### Scenario: Route HTTP Ingress to a multi-port Service

- **WHEN** a multi-port Service has at least one TCP port and an HTTP Ingress endpoint
- **THEN** the platform SHALL route the Ingress to the first declared TCP Service port

#### Scenario: Reject HTTP Ingress without a TCP port

- **WHEN** a multi-port Service has no TCP port and an HTTP Ingress endpoint
- **THEN** the platform SHALL reject the configuration before publication

### Requirement: Application templates project configuration as files

The platform SHALL allow a template to mount declared same-namespace ConfigMap or Secret keys as read-only files in the primary container.

#### Scenario: Mount a Secret key as a file

- **WHEN** a template declares a Secret source, key and absolute mount path
- **THEN** the rendered Pod SHALL include a read-only Secret volume and matching volume mount
- **AND THEN** the value SHALL not be returned in template, release or operation-log responses

#### Scenario: Mount a ConfigMap key as a file

- **WHEN** a template declares a ConfigMap source, key and absolute mount path
- **THEN** the rendered Pod SHALL include a read-only ConfigMap volume and matching volume mount

#### Scenario: Reject unsafe projections

- **WHEN** a file mount uses an invalid resource name, a non-absolute path, a duplicate mount path, or a source outside the application namespace
- **THEN** the platform SHALL reject the template before resources are applied
