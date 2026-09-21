# cluster-platform-identification Specification

## Purpose
TBD - created by archiving change generalize-kubernetes-platform-and-remove-global-tailscale. Update Purpose after archive.
## Requirements
### Requirement: Identify the connected Kubernetes platform

The system SHALL derive the connected cluster distribution from a bounded Kubernetes discovery server-version request and expose `kubernetes`, `k3s`, or `unknown` together with a normalized version string.

#### Scenario: Identify a K3s distribution

- **WHEN** Kubernetes discovery returns a successful server version containing a recognized K3s distribution marker
- **THEN** the system SHALL return distribution `k3s`, the normalized server Git version, and K3s-only capability flags
- **AND THEN** it SHALL NOT require SSH access, a Tailscale address, or a host-local K3s process to make the classification

#### Scenario: Identify a standard Kubernetes distribution

- **WHEN** Kubernetes discovery returns a successful server version without a recognized K3s distribution marker
- **THEN** the system SHALL return distribution `kubernetes` and the normalized server Git version
- **AND THEN** it SHALL mark K3s-only capabilities unavailable

#### Scenario: Discovery is unavailable or unclassifiable

- **WHEN** the discovery request fails, exceeds its fixed deadline, or returns no usable version
- **THEN** the system SHALL return distribution `unknown` with a sanitized availability reason
- **AND THEN** it SHALL not enable K3s-only mutating actions

### Requirement: Present platform identity and feature availability

The frontend SHALL display the current cluster as Kubernetes, K3s, or unknown and SHALL use the returned capability flags to gate K3s-only actions.

#### Scenario: User views a connected K3s cluster

- **WHEN** the identity API reports `k3s`
- **THEN** the frontend SHALL show K3s as the active platform and make supported K3s-specific actions available

#### Scenario: User views a Kubernetes or unknown cluster

- **WHEN** the identity API reports `kubernetes` or `unknown`
- **THEN** the frontend SHALL retain generic Kubernetes management views
- **AND THEN** it SHALL not offer K3s node bootstrap or other K3s-only mutation actions

