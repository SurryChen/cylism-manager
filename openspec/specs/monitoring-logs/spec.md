# monitoring-logs Specification

## Purpose
TBD - created by archiving change monitoring-logs. Update Purpose after archive.
## Requirements
### Requirement: Managed container log collection

The system SHALL install and reconcile a platform-managed Loki StatefulSet and Grafana Alloy DaemonSet in the `monitoring` namespace. Alloy SHALL collect Kubernetes container stdout/stderr from the standard CRI pod and container log paths and forward it only to the cluster-internal Loki Service. Loki SHALL use one replica and a platform-managed `ReadWriteOnce` PVC mounted at `/loki`.

#### Scenario: Install log collection on a ready node
- **WHEN** an administrator selects a ready node, a valid storage capacity, optional StorageClass and a valid retention period
- **THEN** the system creates or reconciles Loki, its Service and PVC, and reconciles the Alloy DaemonSet and its least-privilege Kubernetes access resources
- **AND** the response reports installation progress without exposing a Loki endpoint outside the cluster

#### Scenario: Reject an unavailable storage node
- **WHEN** an administrator requests log collection using a missing or not-ready node
- **THEN** the system rejects the request with an actionable validation error
- **AND** it does not create or relocate the Loki PVC

#### Scenario: Preserve log data on uninstall
- **WHEN** an administrator uninstalls managed log collection
- **THEN** the system removes managed workloads, Service, configuration and collection access resources
- **AND** the Loki data PVC remains available for a subsequent managed installation

### Requirement: Controlled log metadata

The system SHALL attach controlled Kubernetes and platform metadata labels to collected lines, including namespace, pod, container, node, workload identity and available Cylism project, environment and release identifiers. The backend SHALL validate platform application identifiers and map them to the corresponding namespace and workload selector. The system MUST NOT index arbitrary request values, Pod UIDs or user-provided free-text values as Loki labels.

#### Scenario: Collect a platform-managed application log
- **WHEN** a Pod created by a Cylism application emits a stdout or stderr line
- **THEN** the collected record is queryable with the Pod namespace, container, node and available Cylism application scope labels

#### Scenario: Collect an unmanaged workload log
- **WHEN** a Kubernetes workload not created by Cylism emits a stdout or stderr line
- **THEN** the collected record remains queryable by Kubernetes namespace, Pod, container and node labels
- **AND** no fabricated application scope label is added

### Requirement: Managed Loki storage lifecycle

The system SHALL classify `monitoring/cylism-loki-data` as infrastructure storage owned by Loki. The claim SHALL be visible in storage inventory with owner, capacity, StorageClass, phase and usage where available. Generic PVC mutation and data-operation endpoints MUST reject the claim and identify Monitoring Logs as its owner.

#### Scenario: List Loki storage
- **WHEN** an administrator opens the infrastructure storage inventory after installing logs
- **THEN** the Loki PVC displays as infrastructure storage with a navigation action to Monitoring Logs

#### Scenario: Reject a generic operation for Loki storage
- **WHEN** a caller attempts to resize, delete, import, back up, restore or migrate the Loki PVC through a generic storage endpoint
- **THEN** the API rejects the operation without changing the PVC or its workload

### Requirement: Bounded log query API

The system SHALL expose authenticated log status, installation, configuration, filter-option and query APIs through the Cylism backend. The backend SHALL build the Loki query from structured filters and MUST NOT accept raw LogQL from browsers. A query SHALL enforce a maximum 24-hour range, 500 returned lines, 256-character keyword and 10-second upstream timeout. The API SHALL accept either a preset time range or an exact UTC start/end range. Keyword expressions MAY use quoted strings with `\"`, `\\` and `\/` escapes plus bounded `AND` and `OR` operators; the backend SHALL parse them into a limited number of safe Loki line-filter branches and merge duplicate results.

#### Scenario: Search recent logs by application and keyword
- **WHEN** an administrator submits a valid time range, application scope and keyword
- **THEN** the backend validates the application scope, performs a bounded Loki query and returns timestamped log lines with a concise metadata label summary

#### Scenario: Reject excessive query scope
- **WHEN** a caller requests more than 24 hours of logs, more than 500 lines, or a keyword longer than 256 characters
- **THEN** the API rejects the request with an actionable validation error
- **AND** it does not send the query to Loki

#### Scenario: Search an exact time range with a boolean expression
- **WHEN** an administrator submits a valid start/end range shorter than 24 hours and a valid expression such as `"fetch failed" AND timeout OR "connection refused"`
- **THEN** the backend sends only the bounded AND branches to Loki using the exact UTC range
- **AND** it merges, deduplicates and reverse-sorts matching lines before applying the returned-line limit

#### Scenario: Report unavailable logging infrastructure
- **WHEN** Loki or required collection resources are unavailable during a query
- **THEN** the API returns an actionable unavailable error rather than an empty result set

### Requirement: Monitoring log workspace

The monitoring page SHALL present a top-level Logs tab with managed installation/status controls, storage and retention summary, structured filters, keyword search and reverse-chronological log lines. The page SHALL not issue a full log query merely when the tab is opened.

#### Scenario: Search logs from the monitoring page
- **WHEN** an administrator opens the ready Logs tab, selects filters and requests a query
- **THEN** the page renders loading, empty, error and result states without navigating away from Monitoring

#### Scenario: Configure retention
- **WHEN** an administrator changes the retention period to a valid supported value in the Logs settings modal
- **THEN** the platform updates managed Loki configuration and reports that existing data beyond the selected retention will be cleaned by Loki

#### Scenario: Open logging settings
- **WHEN** an administrator opens Logs settings
- **THEN** the platform renders a centered modal above the global navigation, consistent with alert settings

