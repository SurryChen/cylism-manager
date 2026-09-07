# managed-oci-registry Specification

## Purpose
TBD - created by archiving change managed-oci-registry. Update Purpose after archive.
## Requirements
### Requirement: Create one managed persistent OCI Registry

The system SHALL allow an authorized user to create one platform-managed OCI Registry with a Registry image reference, a unique endpoint authority, a data-node selector, an existing eligible PVC, CPU and memory requests/limits, and Basic Auth credentials. The Registry namespace SHALL be fixed to `cylism-system`. The system SHALL create only labeled, platform-owned Kubernetes resources and SHALL persist the desired configuration without storing the submitted plaintext password in an API response or operation log.

#### Scenario: Create a secured Registry

- **WHEN** an authorized user submits a valid HTTPS endpoint, eligible PVC, data node, Registry image, CPU/memory resources and Basic Auth credentials
- **THEN** the system SHALL create a single-replica Registry Deployment, ClusterIP Service, authentication Secret and host-based Ingress in `cylism-system`
- **AND THEN** the Deployment SHALL be scheduled only to the selected data node and mount the PVC at `/var/lib/registry`
- **AND THEN** the Deployment SHALL contain the submitted CPU and memory requests and limits
- **AND THEN** the API response SHALL report credential configuration without returning the password

#### Scenario: Reject an unsafe or conflicting resource definition

- **WHEN** a request uses an endpoint with a path, an invalid resource quantity, a duplicate endpoint, or a Kubernetes resource name already owned by another controller
- **THEN** the system SHALL reject the request before changing Kubernetes resources

### Requirement: Use a node-local PVC safely

The system SHALL use only a user-selected existing PVC in `cylism-system` that has the `local-path` StorageClass and `ReadWriteOnce` access mode. It SHALL require `volumeBindingMode=WaitForFirstConsumer`, never create or modify the selected PVC, and preserve it when the managed Registry is deleted.

#### Scenario: Provision the PVC on the selected data node

- **WHEN** the selected Pending `local-path` PVC has `WaitForFirstConsumer` binding and the Registry Deployment is created with its data-node selector
- **THEN** Kubernetes SHALL provision and bind the node-local PV only after the Registry is scheduled to the selected data node

#### Scenario: Reuse a bound PVC only on its bound node

- **WHEN** the selected PVC is already Bound to a local node
- **THEN** the system SHALL schedule the Registry only to that node and reject a conflicting data-node selector

#### Scenario: Reject unsafe local-path provisioning

- **WHEN** the selected PVC is absent, uses another StorageClass, lacks `ReadWriteOnce`, is referenced by another workload, or its `local-path` StorageClass does not use `WaitForFirstConsumer`
- **THEN** the system SHALL reject Registry creation before creating a Registry workload

### Requirement: Protect the Registry transport mode

The system SHALL require an explicit transport mode. HTTPS mode SHALL require a platform-managed, Ready Certificate in the Registry namespace whose DNS names cover the Registry hostname; the system SHALL derive the referenced TLS Secret from that Certificate and SHALL not accept a user-supplied cross-namespace or arbitrary Secret reference. HTTP mode SHALL require explicit insecure confirmation and SHALL be marked as insecure in all Registry status responses.

#### Scenario: Require a matching managed certificate for HTTPS endpoint

- **WHEN** a user selects HTTPS mode without a Ready platform Certificate in the Registry namespace that covers the endpoint hostname
- **THEN** the system SHALL reject the Registry creation or update
- **AND THEN** the UI SHALL offer only matching certificates and link to certificate management when none exists

#### Scenario: Require explicit confirmation for HTTP endpoint

- **WHEN** a user selects HTTP mode without setting insecure confirmation
- **THEN** the system SHALL reject the Registry creation or update
- **AND THEN** it SHALL explain that HTTP exposes image and credential traffic

### Requirement: Synchronize a managed Registry with image distribution configuration

The system SHALL create or update a platform-owned Image Registry record and Node Registry Mirror record for a successfully configured managed Registry. The records SHALL use the managed endpoint and pull credentials, and SHALL not overwrite a same-endpoint record that is not owned by the managed Registry.

#### Scenario: Apply Registry access to selected nodes

- **WHEN** a user selects one or more cluster nodes and requests Registry access application
- **THEN** the system SHALL apply the managed endpoint, pull credentials and selected TLS/HTTP policy to only those nodes
- **AND THEN** it SHALL report independent per-node status without returning the credential

#### Scenario: Preserve user-managed endpoint records

- **WHEN** the endpoint is already represented by an Image Registry or Node Registry Mirror not associated with the managed Registry
- **THEN** the system SHALL block synchronization and identify the conflicting record

### Requirement: Observe Registry readiness and storage locality

The system SHALL report desired configuration, Registry workload readiness, endpoint health, selected data node, PVC name/capacity/phase, CPU and memory resources, endpoint transport mode and node configuration application status. Endpoint health SHALL treat an authenticated Registry V2 `401 Unauthorized` response as reachable.

#### Scenario: Report a healthy authenticated Registry

- **WHEN** the Registry Pod is ready and its `/v2/` endpoint responds with the expected authentication challenge
- **THEN** the system SHALL report Registry status as ready

#### Scenario: Report data node unavailability

- **WHEN** the selected data node is not Ready or the Registry Pod cannot schedule there
- **THEN** the system SHALL report Registry status as degraded with the scheduling or node reason

### Requirement: Delete without deleting Registry data or active references

The system SHALL require explicit deletion confirmation and SHALL never delete the managed Registry PVC. It SHALL block deletion while an existing Release or Project default references its platform-owned Image Registry, unless the caller removes those references first.

#### Scenario: Block deletion while a Release references the Registry

- **WHEN** a user requests deletion and a Release references the managed Image Registry
- **THEN** the system SHALL reject deletion and identify that active references must be removed first

#### Scenario: Remove only managed resources after confirmation

- **WHEN** a user confirms deletion and no protected reference remains
- **THEN** the system SHALL remove only its labeled Kubernetes resources and associated platform-owned Registry records
- **AND THEN** it SHALL retain the Registry PVC and record that persistent data remains on the selected node

