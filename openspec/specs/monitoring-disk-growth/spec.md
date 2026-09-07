# monitoring-disk-growth Specification

## Purpose
TBD - created by archiving change monitoring-disk-growth. Update Purpose after archive.
## Requirements
### Requirement: Disk growth diagnostics

The system SHALL expose an authenticated, bounded disk-growth diagnostic API backed by existing VictoriaMetrics node, kubelet and cAdvisor metrics. The API SHALL rank positive growth over a supported time range for node mount points, PVCs and container filesystems, without accepting raw PromQL.

#### Scenario: Diagnose cluster-wide disk growth
- **WHEN** an administrator requests a valid time range without a node filter
- **THEN** the system returns bounded rankings for node mount points, PVCs and container filesystems
- **AND** each row identifies the relevant Kubernetes labels and positive byte growth

#### Scenario: Diagnose one node
- **WHEN** an administrator selects a valid node and time range
- **THEN** the system applies the node constraint to every diagnostic query
- **AND** it returns only the matching node's ranked growth rows

### Requirement: PVC consumer context

The system SHALL enrich PVC growth rows with Pods that currently reference the claim in the same namespace.

#### Scenario: Show a growing application volume
- **WHEN** a PVC growth metric belongs to a claim currently mounted by one or more Pods
- **THEN** the result identifies the namespace, claim and current Pod consumers

### Requirement: Monitoring disk workspace

The monitoring page SHALL provide a lazily loaded Disk tab where administrators can select a supported time range and optional node, view node mount-point, PVC and container growth rankings, and refresh the diagnostic results.

#### Scenario: Open the disk workspace
- **WHEN** an administrator selects the Disk tab after monitoring is ready
- **THEN** the page loads the disk diagnostic once and renders loading, empty and error states
- **AND** opening other monitoring tabs does not load disk diagnostics

