# infrastructure-storage Specification

## Purpose
TBD - created by archiving change infrastructure-pvc-management. Update Purpose after archive.
## Requirements
### Requirement: VictoriaMetrics managed persistent storage

The system SHALL create and reconcile `monitoring/cylism-victoria-metrics-data` for every new VictoriaMetrics installation. The claim SHALL be platform-managed, labelled as VictoriaMetrics infrastructure storage, use `ReadWriteOnce`, and be mounted by the VictoriaMetrics container at `/storage`; the deployment SHALL use the selected node's hostname nodeSelector before the claim is first consumed.

#### Scenario: Install VictoriaMetrics with a local-path PVC
- **WHEN** an administrator selects a ready node, a valid positive capacity, an available StorageClass or the default StorageClass, and retention settings for a new installation
- **THEN** the system creates or reconciles the VictoriaMetrics infrastructure PVC and deployment
- **AND** the deployment mounts the PVC at `/storage` with `kubernetes.io/hostname` set to the selected node
- **AND** the installation status reports the requested storage capacity, StorageClass and actual bound node when available

#### Scenario: Reject an invalid storage request
- **WHEN** an administrator submits a missing or non-positive capacity, an invalid capacity unit, an unavailable StorageClass, or a non-ready node
- **THEN** the system rejects the request with an actionable validation error
- **AND** the system does not create or change the VictoriaMetrics workload or PVC

### Requirement: Legacy VictoriaMetrics hostPath migration

The system SHALL recognize an existing VictoriaMetrics deployment that mounts a hostPath as a legacy storage mode. The system MUST preserve that hostPath and its data during platform upgrades and non-storage configuration updates, and SHALL provide an explicit same-node migration to a system-managed PVC.

#### Scenario: Read a legacy installation
- **WHEN** the VictoriaMetrics deployment contains the existing hostPath storage volume
- **THEN** the status API reports legacy hostPath storage including the resolved node and path
- **AND** the monitoring UI identifies it as legacy storage instead of presenting it as a PVC-backed installation

#### Scenario: Update retention for a legacy installation
- **WHEN** an administrator changes only the retention setting of a legacy hostPath installation
- **THEN** the system updates the VictoriaMetrics arguments while retaining the existing hostPath volume and node selector
- **AND** the system does not create, attach, or delete a VictoriaMetrics PVC

#### Scenario: Migrate a legacy hostPath to a managed PVC
- **WHEN** an administrator explicitly starts migration for a legacy VictoriaMetrics installation whose selected node is ready and whose source directory is accessible
- **THEN** the system stops the VictoriaMetrics workload, automatically creates or reconciles its managed PVC, and runs a node-constrained migration Job that copies and verifies the hostPath data into the PVC
- **AND** after verification the system updates the deployment to mount the PVC at `/storage`, restores the workload, and reports the result only after the new Pod is ready
- **AND** the original hostPath directory remains intact after a successful migration

#### Scenario: Recover from a failed legacy migration
- **WHEN** the migration copy, verification, deployment switch, or readiness check fails
- **THEN** the system restores the original hostPath deployment and its replica count
- **AND** the system preserves the source directory and the target PVC for diagnosis
- **AND** the migration status reports the failing stage and actionable error detail

### Requirement: Infrastructure storage inventory and operation boundaries

The system SHALL classify platform-owned Alertmanager and VictoriaMetrics PVCs as infrastructure storage in the cluster storage inventory. The API SHALL expose their owner identity, storage mode and read-only operation boundaries; all generic PVC mutation endpoints MUST reject infrastructure PVCs.

#### Scenario: List Alertmanager and VictoriaMetrics PVCs
- **WHEN** an administrator opens cluster storage inventory containing platform-managed Alertmanager or VictoriaMetrics claims
- **THEN** each claim displays an infrastructure owner, namespace, capacity, phase, StorageClass and bound node where available
- **AND** the UI provides an owner-specific navigation action to monitoring or alerting settings
- **AND** the UI does not render generic create, edit, resize, delete, backup, import, restore or migration controls for those claims

#### Scenario: Observe local PVC data usage
- **WHEN** an administrator opens or refreshes cluster storage inventory
- **THEN** the system asynchronously reports directory usage for each Bound local-path or hostPath PVC whose node has a registered SSH server
- **AND** the inventory remains usable when a node is unavailable, a PVC is not local, or usage collection fails
- **AND** the UI shows collected usage against the PVC requested capacity and renders the infrastructure monitoring link without an underline

#### Scenario: Reject generic operation for infrastructure PVC
- **WHEN** a caller invokes a generic PVC create, update, resize, delete, backup, import, restore or migration API for an Alertmanager or VictoriaMetrics infrastructure PVC
- **THEN** the API rejects the operation with an error explaining that the PVC is managed by the corresponding infrastructure component
- **AND** the system leaves the PVC and its workload unchanged

### Requirement: System-owned infrastructure PVC lifecycle

The system SHALL create and reconcile Alertmanager and VictoriaMetrics data PVCs only through their owning infrastructure component. The system MUST preserve existing infrastructure PVC data and prohibit users from changing their claim specification or lifecycle through the generic storage interface.

#### Scenario: Reconcile an existing Alertmanager PVC
- **WHEN** Alertmanager is installed or reconfigured and `cylism-alertmanager-data` already exists
- **THEN** the system preserves the claim specification and data
- **AND** the system adds or updates only the platform ownership labels needed for infrastructure inventory

#### Scenario: Protect an infrastructure PVC from generic update
- **WHEN** a caller attempts to change the capacity, StorageClass, labels, lifecycle or data-operation settings of an Alertmanager or VictoriaMetrics PVC through the storage API
- **THEN** the system rejects the request and identifies the owning infrastructure component
- **AND** the PVC specification and mounted workload remain unchanged

#### Scenario: Uninstall Alertmanager
- **WHEN** an administrator uninstalls managed alerting
- **THEN** the system removes Alertmanager workloads and nonpersistent configuration as defined by alerting lifecycle
- **AND** the Alertmanager PVC remains available in storage inventory as infrastructure storage

