## ADDED Requirements

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

## REMOVED Requirements

### Requirement: Detect K3s Tailscale mode from active configuration
**Reason**: A K3s VPN marker does not by itself prove a Tailscale-specific embedded mode; diagnostics now report K3s VPN integration with safe provider classification.

**Migration**: Clients replace `k3s_embedded_tailscale`, `external_tailscale`, and `standard_network` mode handling with the K3s VPN integration status and cluster platform identity contracts.

### Requirement: Collect bounded Tailscale local-network evidence
**Reason**: Generic platform behavior must not depend on Tailscale CLI availability or Tailnet state.

**Migration**: Use the K3s VPN compatibility result for K3s configuration evidence; inspect Tailscale runtime state outside Cylism Manager when needed.

### Requirement: Diagnose actual Tailnet peer paths
**Reason**: Peer probing requires a Tailscale runtime and exposes a provider-specific operational concern outside the platform boundary.

**Migration**: Use operator-approved external network observability tooling for Tailnet path diagnosis.

### Requirement: Present network diagnostics in server management
**Reason**: The prior view presented Tailnet modes and peer paths; it is replaced by K3s-scoped VPN compatibility presentation.

**Migration**: Clients render the new K3s VPN compatibility fields and omit Tailnet-specific tables and columns.
