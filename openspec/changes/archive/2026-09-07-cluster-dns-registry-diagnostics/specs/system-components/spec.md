## MODIFIED Requirements

### Requirement: The platform manages CoreDNS rollout and node placement

The system SHALL save and directly apply CoreDNS replicas, RollingUpdate settings, and an optional `kubernetes.io/hostname` node selector only when the CoreDNS availability profile supports the requested action and its preconditions pass. It SHALL only accept a Ready, schedulable cluster node as a placement target and SHALL periodically re-apply the saved configuration while that mode remains valid.

#### Scenario: Apply a supported CoreDNS high-availability baseline

- **WHEN** an administrator applies the CoreDNS high-availability baseline and at least two Ready, schedulable nodes satisfy the profile preflight
- **THEN** the system SHALL set two replicas with `maxUnavailable: 0` and `maxSurge: 1` in the Deployment rolling-update strategy
- **AND THEN** it SHALL report the exact profile and preflight evidence used for the change

#### Scenario: Block an unsafe CoreDNS replica increase

- **WHEN** the CoreDNS high-availability preflight finds insufficient nodes, unavailable resources, an unhealthy Deployment, or a managed image-pull blocker
- **THEN** the system SHALL reject the baseline without changing the Deployment
- **AND THEN** it SHALL return a bounded explanation of the failed prerequisite

#### Scenario: Migrate CoreDNS to a fixed node

- **WHEN** an administrator selects a different Ready, schedulable node and saves CoreDNS configuration
- **THEN** the system SHALL set `nodeSelector.kubernetes.io/hostname` on the Deployment template
- **AND THEN** the Deployment controller rolls CoreDNS Pods onto that node using the configured rollout strategy

### Requirement: System component safety baselines are profile-driven

The system SHALL select replica and rollout safety actions from an explicit availability profile for each whitelisted system component. It SHALL NOT infer high-availability support solely from `static_deployment` controller mode.

#### Scenario: Unsupported static component baseline

- **WHEN** an administrator opens a static component whose availability profile does not support replica scaling, including `metrics-server` or `local-path-provisioner`
- **THEN** the system SHALL show that its replica policy is managed by K3s or the component profile
- **AND THEN** it SHALL not present an action that increases replicas to two

#### Scenario: Component has no availability profile

- **WHEN** an administrator opens a whitelisted component with no explicit availability profile
- **THEN** the system SHALL not present a generic replica safety baseline
- **AND THEN** it MAY expose only read-only state and actions independently declared safe for that component

#### Scenario: Restore an unsupported existing replica count

- **WHEN** an unsupported component currently differs from its profile default and an administrator explicitly confirms restore-default
- **THEN** the system SHALL restore only the profile-controlled fields for that component
- **AND THEN** it SHALL not apply a DNS policy change or modify a different component
