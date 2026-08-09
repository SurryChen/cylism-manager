## ADDED Requirements

### Requirement: The platform persists built-in chart configuration via HelmChartConfig

The system SHALL apply built-in K3s chart values through the HelmChartConfig CRD so changes survive K3s re-rendering and upgrades. It SHALL NOT modify the Deployment object directly.

#### Scenario: Save chart values
- **WHEN** an administrator saves values for a whitelisted system chart
- **THEN** the system creates or updates the corresponding HelmChartConfig CRD
- **AND THEN** the values are persisted in the Manager database with apply status

#### Scenario: Restore chart defaults
- **WHEN** an administrator reverts a system chart
- **THEN** the system deletes the HelmChartConfig CRD
- **AND THEN** K3s restores the bundled chart defaults

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

#### Scenario: Apply safe rollout baseline
- **WHEN** an administrator applies the safe rollout baseline to a component
- **THEN** the system generates values with `maxUnavailable: 0` and `maxSurge: 1` and persists them
