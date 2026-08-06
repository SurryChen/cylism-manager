## ADDED Requirements

### Requirement: Dedicated assistant Runtime namespace

The system SHALL reconcile the platform-managed assistant Runtime's Namespace, Deployment, Service, ConfigMap, Secret, and audit PVC in the `cylism-assistant` namespace.

#### Scenario: Install a Runtime with no legacy installation

- **WHEN** an operator deploys the assistant Runtime and no legacy Runtime exists in `default`
- **THEN** the system creates or updates the Runtime resources in `cylism-assistant`
- **AND** it reports status from the Deployment and audit PVC in `cylism-assistant`

#### Scenario: Use default in-cluster Runtime service discovery

- **WHEN** `CYLISM_ASSISTANT_RUNTIME_URL` is unset
- **THEN** the Manager sends Runtime requests to `cylism-ops-agent.cylism-assistant.svc` on port `8080`

### Requirement: Managed legacy Runtime audit migration

The system SHALL preserve audit data when moving the legacy assistant Runtime from `default` to `cylism-assistant`, using a persisted, controlled migration workflow initiated by an explicit deploy-or-update operation.

#### Scenario: Start a legacy audit migration

- **WHEN** an operator deploys or updates the Runtime and `default/cylism-ops-agent-audit` exists
- **THEN** the system persists a Runtime migration stage
- **AND** it stops the legacy Runtime before copying its audit files to a bound PVC in `cylism-assistant`
- **AND** it reports the migration stage through Runtime status

#### Scenario: Verify copied audit storage

- **WHEN** audit data is copied to the target PVC
- **THEN** the system verifies the checksum of each existing SQLite audit file before starting the target Runtime
- **AND** it retains the legacy audit PVC when a copy or verification failure occurs

#### Scenario: Complete verified cutover

- **WHEN** the target Runtime has completed audit verification and reports ready in `cylism-assistant`
- **THEN** the system deletes the named legacy Deployment, Service, ConfigMap, Secret, temporary binding Pod, and `default/cylism-ops-agent-audit` PVC
- **AND** missing legacy resources do not cause the cleanup to fail

#### Scenario: Resume an interrupted migration

- **WHEN** a migration remains incomplete after a Manager restart or transient failure
- **THEN** a later deploy-or-update operation resumes or recovers the persisted migration stage
- **AND** it does not delete the source audit PVC before verification and target Runtime readiness

### Requirement: Retain active Runtime audit storage on uninstall

The system SHALL retain the audit PVC in `cylism-assistant` when an operator uninstalls the Runtime.

#### Scenario: Uninstall the active Runtime

- **WHEN** an operator uninstalls the assistant Runtime
- **THEN** the system removes the active Deployment, Service, ConfigMap, and Secret from `cylism-assistant`
- **AND** it does not delete `cylism-assistant/cylism-ops-agent-audit`

### Requirement: Migrate an active Runtime between nodes

The system SHALL support an explicit, controlled move of a ready assistant Runtime and its audit storage to a different ready node in `cylism-assistant`.

#### Scenario: Request a Runtime node migration

- **WHEN** an operator requests a Runtime node migration to a different ready node
- **THEN** the system creates a uniquely named target audit PVC on that node
- **AND** it reuses the managed audit transfer and verification workflow before changing the Deployment's node selector and claim reference

#### Scenario: Preserve the active migrated claim

- **WHEN** the target Runtime becomes ready after a node migration
- **THEN** the system treats the generated target PVC as the active Runtime audit claim
- **AND** later Runtime updates and uninstalls use that active claim rather than recreating the original fixed claim

#### Scenario: Reinstall after retaining a migrated claim

- **WHEN** an operator uninstalls a Runtime whose generated migrated audit PVC was active and later deploys it again
- **THEN** the system reuses the retained active migrated PVC
- **AND** it does not create a new fixed-name audit PVC

#### Scenario: Reject an incidental node change

- **WHEN** an operator uses ordinary deploy-or-update with a node different from the active local audit PVC's bound node
- **THEN** the system rejects the request and directs the operator to the explicit Runtime storage migration action
