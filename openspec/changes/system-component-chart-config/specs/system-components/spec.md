## ADDED Requirements

### Requirement: The platform persists built-in chart configuration via HelmChartConfig

The system SHALL apply built-in K3s chart values through the HelmChartConfig CRD when the component is Helm-managed. CoreDNS SHALL be handled separately because current K3s releases provide it as a static Deployment manifest.

#### Scenario: Save chart values
- **WHEN** an administrator saves values for a Helm-managed whitelisted system chart
- **THEN** the system creates or updates the corresponding HelmChartConfig CRD
- **AND THEN** the values are persisted in the Manager database with apply status

#### Scenario: Restore chart defaults
- **WHEN** an administrator reverts a Helm-managed system chart
- **THEN** the system deletes the HelmChartConfig CRD
- **AND THEN** K3s restores the bundled chart defaults

### Requirement: The platform selects a component control adapter from runtime evidence

The system SHALL detect the current control source before writing a whitelisted component. A matching HelmChart SHALL select `helm_chart`; a same-name Deployment without a matching HelmChart SHALL select `static_deployment`; an in-process K3s component SHALL select `embedded`; insufficient evidence SHALL select `unknown`.

#### Scenario: Detect Helm-managed component
- **WHEN** a whitelisted component has a matching HelmChart
- **THEN** the system reports `controller_mode: helm_chart`
- **AND THEN** it writes configuration only through HelmChartConfig

#### Scenario: Detect static Deployment component
- **WHEN** a whitelisted component has no matching HelmChart and has a same-name Deployment
- **THEN** the system reports `controller_mode: static_deployment`
- **AND THEN** it applies only platform-managed Deployment fields

#### Scenario: Reject an unknown control source
- **WHEN** a whitelisted component cannot be matched to a HelmChart, static Deployment, or supported in-process component
- **THEN** the system reports `controller_mode: unknown`
- **AND THEN** it rejects configuration writes without creating a HelmChartConfig

### Requirement: The platform manages CoreDNS rollout and node placement

The system SHALL save and directly apply CoreDNS replicas, RollingUpdate settings, and an optional `kubernetes.io/hostname` node selector when runtime detection selects `static_deployment`. It SHALL only accept a Ready, schedulable cluster node as a placement target and SHALL periodically re-apply the saved configuration while that mode remains valid.

#### Scenario: Apply CoreDNS safe rollout baseline
- **WHEN** an administrator applies the CoreDNS safe rollout baseline
- **THEN** the system sets two replicas with `maxUnavailable: 0` and `maxSurge: 1` in the Deployment rolling-update strategy

#### Scenario: Migrate CoreDNS to a fixed node
- **WHEN** an administrator selects a different Ready, schedulable node and saves CoreDNS configuration
- **THEN** the system sets `nodeSelector.kubernetes.io/hostname` on the Deployment template
- **AND THEN** the Deployment controller rolls CoreDNS Pods onto that node using the configured rollout strategy

#### Scenario: Restore CoreDNS defaults
- **WHEN** an administrator restores default CoreDNS configuration
- **THEN** the system restores one replica, the K3s default rolling-update settings, and removes the hostname selector

### Requirement: Only known system charts can be managed

The system SHALL reject configuration for chart names outside the built-in whitelist.

#### Scenario: Reject unknown chart
- **WHEN** an administrator submits values for an unknown chart name
- **THEN** the system rejects the request without contacting Kubernetes

#### Scenario: Reject invalid YAML
- **WHEN** an administrator submits malformed values content
- **THEN** the system rejects the request and does not write the CRD

### Requirement: The console shows desired and actual component state

The system SHALL list whitelisted components with their saved values and the current Deployment state (replicas, ready replicas, rollout strategy, image).

#### Scenario: List system components
- **WHEN** an administrator opens the system components tab
- **THEN** the system returns each whitelisted component with saved values and actual Deployment status
