## MODIFIED Requirements

### Requirement: The platform persists built-in chart configuration via HelmChartConfig

The system SHALL apply built-in K3s chart values through the HelmChartConfig CRD when the component is Helm-managed. When the chart is Traefik, the platform SHALL support a structured optional entrypoint request read timeout and render it for both `web` and `websecure` entrypoints. CoreDNS SHALL be handled separately because current K3s releases provide it as a static Deployment manifest.

#### Scenario: Save a Traefik upload read timeout

- **WHEN** an administrator saves a valid `30m` Traefik read timeout
- **THEN** the system creates or updates `kube-system/traefik` HelmChartConfig
- **AND THEN** its values include the `web` and `websecure` `readTimeout=30m` arguments
- **AND THEN** the desired values and apply status are persisted in the Manager database

#### Scenario: Leave the Traefik timeout at its default

- **WHEN** an administrator saves Traefik configuration with no timeout override
- **THEN** the system does not render either managed `readTimeout` argument
- **AND THEN** the console reports that Traefik's default 60-second value applies until an actual override is detected

#### Scenario: Reject an invalid Traefik timeout

- **WHEN** an administrator submits a zero, negative, malformed, or out-of-range Traefik timeout
- **THEN** the system rejects the request before changing HelmChartConfig or the persisted desired configuration

#### Scenario: Restore Traefik defaults

- **WHEN** an administrator restores the Traefik component defaults
- **THEN** the system deletes the Traefik HelmChartConfig and its persisted configuration
- **AND THEN** K3s restores the bundled Traefik chart behavior

### Requirement: The console shows desired and actual component state

The system SHALL list whitelisted components with their saved values and the current Deployment state (replicas, ready replicas, rollout strategy, image). For Helm-managed Traefik, it SHALL additionally show the desired and effective entrypoint request read timeout and distinguish a pending Helm reconciliation from an effective configuration.

#### Scenario: Show pending Traefik timeout application

- **WHEN** a valid Traefik timeout is saved but its Deployment has not yet started with both expected entrypoint arguments
- **THEN** the console shows the desired timeout as waiting to take effect
- **AND THEN** it does not falsely report the timeout as effective

#### Scenario: Show effective Traefik timeout

- **WHEN** the Traefik Deployment container arguments contain the expected timeout for both `web` and `websecure`
- **THEN** the console shows the timeout as effective

#### Scenario: Configure another Helm-managed system chart

- **WHEN** an administrator configures a Helm-managed chart other than Traefik
- **THEN** the console does not offer the Traefik timeout control
- **AND THEN** the API rejects Traefik timeout values for that chart
