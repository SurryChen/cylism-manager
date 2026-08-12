## ADDED Requirements

### Requirement: Manage external CoreDNS forwarding separately from component lifecycle

The system SHALL expose a Cluster DNS management surface that owns only the platform-managed external forwarding policy. It SHALL keep CoreDNS replica count, rollout strategy, and placement in the system-components surface.

#### Scenario: View inherited forwarding

- **WHEN** no platform DNS policy is active
- **THEN** the Cluster DNS page SHALL show that external forwarding is inherited from the active K3s CoreDNS configuration
- **AND THEN** it SHALL show Ready CoreDNS Pod locations and current forwarding evidence

#### Scenario: Keep component controls separate

- **WHEN** an administrator applies a Cluster DNS forwarding policy
- **THEN** the system SHALL NOT change CoreDNS replicas, rollout strategy, or node selector

### Requirement: Validate and apply a versioned resolver policy

The system SHALL accept only structured, bounded resolver settings and SHALL validate every configured resolver from every Ready CoreDNS Pod before applying it through the Manager-owned CoreDNS custom override.

#### Scenario: Apply a validated policy

- **WHEN** an administrator submits valid resolver addresses and every Ready CoreDNS Pod can complete the fixed query set through each resolver
- **THEN** the system SHALL create a new immutable policy revision
- **AND THEN** it SHALL update only the Manager-owned CoreDNS custom override
- **AND THEN** it SHALL start a controlled CoreDNS rollout and expose its status

#### Scenario: Reject an invalid or unreachable resolver

- **WHEN** a submitted resolver is malformed, unsupported, or cannot answer the fixed query set from any Ready CoreDNS Pod
- **THEN** the system SHALL reject the policy without modifying CoreDNS configuration
- **AND THEN** it SHALL return bounded validation evidence identifying the failing Pod and resolver

#### Scenario: Roll back to a previous policy

- **WHEN** an administrator selects a prior policy revision to roll back
- **THEN** the system SHALL revalidate that revision against current Ready CoreDNS Pods
- **AND THEN** it SHALL apply the selected values as a new active revision only after validation succeeds

#### Scenario: Restore inherited forwarding

- **WHEN** an administrator removes the active platform DNS policy
- **THEN** the system SHALL remove only the Manager-owned CoreDNS custom override
- **AND THEN** it SHALL restore inherited K3s forwarding without replacing the base Corefile

### Requirement: Surface resolver consistency evidence

The system SHALL record and display bounded resolver health evidence for each Ready CoreDNS Pod and configured resolver.

#### Scenario: Inconsistent answers

- **WHEN** Ready CoreDNS Pods return materially different reachable results for an allowlisted registry hostname
- **THEN** the system SHALL report `dns_resolution_inconsistent`
- **AND THEN** it SHALL display the Pod-scoped answer sets without exposing unrelated resolver configuration
