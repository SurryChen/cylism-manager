# monitoring-alerting Specification

## Purpose
TBD - created by archiving change monitoring-alerting. Update Purpose after archive.
## Requirements
### Requirement: Managed alerting installation

The system SHALL install and reconcile a platform-managed vmalert and Alertmanager in the `monitoring` namespace only when VictoriaMetrics is ready. Alertmanager SHALL use one replica, a 1Gi `ReadWriteOnce` PVC, and the selected ready node's hostname label; vmalert SHALL query the platform-managed VictoriaMetrics Service every minute.

#### Scenario: Install alerting on a ready node
- **WHEN** an administrator selects a ready alerting node and submits a valid configuration while VictoriaMetrics is ready
- **THEN** the system creates or updates the managed alerting resources and returns their installation status without exposing notification secrets

#### Scenario: Reject installation before metrics are ready
- **WHEN** an administrator requests alerting installation while VictoriaMetrics is not ready
- **THEN** the system rejects the request with an actionable conflict message and does not create alerting workloads

### Requirement: Managed default alert rules

The system SHALL render and evaluate platform-managed rules for node reachability, CPU, memory, root disk, Pod restart/Pending state, workload replica availability, and monitoring component availability. Each rule SHALL support an enabled state, threshold where applicable, and a sustained duration; invalid values SHALL be rejected before configuration is applied.

#### Scenario: Update an alert threshold
- **WHEN** an administrator changes a valid enabled rule threshold or duration
- **THEN** the system updates the managed rule configuration and rolls vmalert so the revised rule is evaluated

#### Scenario: Disable a default rule
- **WHEN** an administrator disables a managed rule
- **THEN** the system omits that rule from the rendered rule file and no longer emits new alerts for it

### Requirement: Configurable notification timing

The system SHALL allow administrators to configure Alertmanager's initial notification wait, interval for new alerts in an existing group, and repeat interval for unresolved alerts. The system SHALL keep `group_by` labels platform-managed and SHALL reject timing values outside supported ranges. Existing configurations without these values SHALL use defaults of 30 seconds, 5 minutes, and 240 minutes respectively.

#### Scenario: Update notification timing
- **WHEN** an administrator saves notification timing values within the supported ranges
- **THEN** the system renders the selected durations into the managed Alertmanager configuration while retaining the platform-managed grouping labels

#### Scenario: Reject an invalid notification interval
- **WHEN** an administrator submits a notification timing value outside the supported range
- **THEN** the system rejects the configuration with an actionable validation message and preserves the previously applied Alertmanager configuration

### Requirement: Secure Feishu notification channel

The system SHALL store the Feishu robot Webhook URL and Alertmanager-to-platform callback token only in a Kubernetes Secret. The configuration API SHALL report only whether the channel is configured, and Alertmanager notifications SHALL be accepted by the platform only with the configured bearer token.

#### Scenario: Send a test notification
- **WHEN** an administrator requests a test after configuring a valid Feishu Webhook URL
- **THEN** the system sends a recognizable test message and reports the delivery outcome without returning the Webhook URL

#### Scenario: Reject an untrusted internal notification
- **WHEN** the notification callback lacks a valid bearer token
- **THEN** the system rejects the callback and does not forward any message to Feishu

### Requirement: Alert operations API

The system SHALL expose managed alerting status, active alerts, recent resolved alerts, current silences, and silence creation/deletion APIs. Active alerts SHALL preserve their severity, labels, annotations, current state, start time, and generator URL; the platform SHALL retain at most 12 successfully forwarded resolved events in memory. API errors from Alertmanager SHALL be surfaced as actionable platform errors.

#### Scenario: Create a silence for an active alert
- **WHEN** an administrator creates a silence with one or more matchers, a supported duration, and an optional comment
- **THEN** the system creates the silence in Alertmanager and returns the resulting silence identifier and expiration time

#### Scenario: Read active alerts while alerting is unavailable
- **WHEN** Alertmanager cannot be reached
- **THEN** the system returns a service error explaining that alerting is unavailable rather than reporting an empty alert list

### Requirement: Secure SMTP email notification channel

The system SHALL support SMTP email notifications through the authenticated Alertmanager-to-platform callback. SMTP credentials SHALL be stored only in a Kubernetes Secret and never returned by the configuration API.

#### Scenario: Send an SMTP test notification
- **WHEN** an administrator configures a valid SMTP endpoint, TLS mode, sender, recipient, and optional credentials then requests a test
- **THEN** the platform SHALL send a recognizable email test notification without exposing SMTP credentials

#### Scenario: Test one configured notification channel
- **WHEN** an administrator requests a Feishu or SMTP test from that channel's configuration section
- **THEN** the platform SHALL send only through the selected configured channel and return a channel-specific result

### Requirement: Contextual notification rendering

The system SHALL render managed alert notifications with the alert rule, affected target, summary, current value when supplied by the rule, threshold when applicable, sustained duration, and start time. Feishu notifications SHALL use an interactive card, and SMTP notifications SHALL use an HTML card with a plain-text alternative. The system SHALL include a platform alert link generated from the server-side platform public URL configuration.

#### Scenario: Send a high disk usage notification
- **WHEN** a managed node disk rule fires with a current metric value and configured threshold
- **THEN** the Feishu card and email display the node, current value, threshold, rule duration, and a link to the platform alert workspace

#### Scenario: Send a contextual test notification
- **WHEN** an administrator tests a configured Feishu or SMTP channel
- **THEN** the system sends a marked test alert using the same contextual card layout and simulated metric fields as a node disk usage alert

#### Scenario: Normalize an invalid platform public URL
- **WHEN** the configured platform public URL is missing or invalid
- **THEN** the system uses the platform default public URL for notification links

### Requirement: Centered alert settings modal

The alerting workspace SHALL present notification and rule configuration in a centered modal above the global application navigation.

#### Scenario: Open alert settings
- **WHEN** an administrator opens alert settings
- **THEN** the platform SHALL render a centered modal above the application navigation

#### Scenario: Display independent notification configuration states
- **WHEN** an administrator opens alert settings after configuring Feishu, SMTP, or both
- **THEN** the workspace SHALL display the configured state independently for each notification channel

### Requirement: Alerting workspace

The monitoring page SHALL provide an Alerting view that prioritizes active alerts by severity, displays alert summary counters, recent resolved alerts, and provides actions to silence or navigate to the affected node or workload. Rule and notification configuration SHALL be presented in a centered modal rather than a second-level page navigation.

#### Scenario: Render active node alert
- **WHEN** Alertmanager reports an active alert with a node label
- **THEN** the workspace displays its severity, summary, current value or description, duration, and an action that opens the Node monitoring view for that node

#### Scenario: Render a healthy alerting state
- **WHEN** alerting is ready and Alertmanager returns no active alerts
- **THEN** the workspace displays a compact healthy state instead of an empty alert table

#### Scenario: Render an active metric alert
- **WHEN** Alertmanager reports an active alert with `current_value` and `threshold` annotations
- **THEN** the workspace displays both values with their rendered units beside the alert summary

#### Scenario: Render recent recoveries
- **WHEN** Alertmanager reports one or more resolved alerts
- **THEN** the workspace displays the recovery table in the shared glass card surface

