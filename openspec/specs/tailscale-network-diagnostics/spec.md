# tailscale-network-diagnostics Specification

## Purpose
TBD - created by archiving change tailscale-network-diagnostics. Update Purpose after archive.

## Requirements

### Requirement: Detect active K3s VPN integration safely

The system SHALL perform K3s VPN compatibility detection only for a server with an active `k3s` or `k3s-agent` unit and only when that active unit or standard K3s configuration declares a supported VPN marker.

#### Scenario: Active K3s VPN configuration is detected

- **WHEN** an active K3s unit or standard K3s configuration declares `vpn-auth`, `vpn-auth-file`, `K3S_VPN_AUTH`, or `K3S_VPN_AUTH_FILE`
- **THEN** the system SHALL return that K3s VPN integration is configured, the active unit name, and a safe provider classification of `tailscale`, `other`, or `unknown`
- **AND THEN** it SHALL NOT return an option value, join key, Auth Key, environment value, configuration-file content, or raw command output

#### Scenario: No active K3s VPN configuration exists

- **WHEN** the server is not running an active K3s unit, or its active K3s configuration has no supported VPN marker
- **THEN** the system SHALL return that K3s VPN integration is not configured
- **AND THEN** it SHALL NOT execute a Tailscale command or return Tailnet, NAT, DERP, peer, or endpoint data

### Requirement: Present K3s VPN compatibility without network mutation

The server-management page SHALL present K3s VPN integration status only for results that report active K3s VPN configuration and SHALL keep the diagnostic read-only.

#### Scenario: Mixed server inventory

- **WHEN** a diagnostic response contains K3s VPN-integrated, standard K3s, Kubernetes, or unavailable servers
- **THEN** the page SHALL show K3s VPN information only for the integrated entries and normal server state for the others
- **AND THEN** a failed collection SHALL not hide successfully collected entries

#### Scenario: User refreshes compatibility diagnostics

- **WHEN** an administrator opens or refreshes network diagnostics
- **THEN** the system SHALL perform only fixed, bounded, read-only checks
- **AND THEN** it SHALL NOT install or configure Tailscale, change K3s, firewall, routing, or node configuration
