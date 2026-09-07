## ADDED Requirements

### Requirement: Detect K3s Tailscale mode from active configuration

The system SHALL determine a server's K3s Tailscale mode from its active K3s service and standard K3s configuration, without inferring the mode from an IP range.

#### Scenario: Active K3s Server uses embedded Tailscale

- **WHEN** the active `k3s` service has `--vpn-auth`, `--vpn-auth-file`, `K3S_VPN_AUTH`, or `K3S_VPN_AUTH_FILE`, or the standard K3s configuration contains the equivalent VPN key
- **THEN** the system SHALL report `k3s_embedded_tailscale` and the active unit name
- **AND THEN** it SHALL NOT return the matching option value, join key, Auth Key, environment value, or configuration-file content

#### Scenario: Tailscale is external to K3s

- **WHEN** Tailscale is installed or online on a server but the active K3s service and K3s configuration have no embedded VPN marker
- **THEN** the system SHALL report `external_tailscale`

#### Scenario: Inactive unit has stale VPN configuration

- **WHEN** an inactive K3s unit has a VPN marker but the active K3s unit and K3s configuration do not
- **THEN** the system SHALL NOT classify the server as `k3s_embedded_tailscale` solely because of the inactive unit

### Requirement: Collect bounded Tailscale local-network evidence

The system SHALL collect structured, bounded Tailscale local state from registered servers that have the Tailscale CLI available.

#### Scenario: Collect online Tailnet state

- **WHEN** a registered server has an online Tailscale instance
- **THEN** the system SHALL report installation state, online state, Tailnet IP, UDP availability, NAT mapping behavior, and nearest DERP region when available
- **AND THEN** it SHALL not return complete Tailscale status JSON, peer metadata, Auth Keys, or public endpoint addresses

#### Scenario: Per-node probe failure

- **WHEN** SSH, K3s, or Tailscale collection fails or exceeds its fixed timeout for one server
- **THEN** the system SHALL return an `unknown` result and a sanitized error category for that server
- **AND THEN** it SHALL continue collecting eligible results from other registered servers

### Requirement: Diagnose actual Tailnet peer paths

The system SHALL probe only eligible, registered Tailnet peers and classify each directed connection by its current Tailscale path.

#### Scenario: Direct UDP path

- **WHEN** a bounded Tailscale ping from one registered node to another reports a direct endpoint path
- **THEN** the system SHALL report the link path as `direct` and include the measured latency when available
- **AND THEN** it SHALL not disclose the endpoint address or port

#### Scenario: DERP relay path

- **WHEN** a bounded Tailscale ping reports a DERP relay path
- **THEN** the system SHALL report the link path as `derp` and include the DERP region and measured latency when available

#### Scenario: Unreachable peer

- **WHEN** a bounded Tailscale ping has no successful response before its deadline
- **THEN** the system SHALL report the link path as `unreachable` without attempting a user-specified host or address

### Requirement: Present network diagnostics in server management

The server-management page SHALL provide a deliberate network-diagnostics view for refreshing and inspecting current K3s/Tailscale mode and node-pair paths.

#### Scenario: Mixed network topology

- **WHEN** the diagnostic result contains embedded Tailscale, external Tailscale, and unknown servers, along with direct or DERP links
- **THEN** the page SHALL show the network mode per server and the path classification per directed link
- **AND THEN** a failed node or link SHALL not hide successful results

#### Scenario: No automatic network mutation

- **WHEN** an administrator opens or refreshes network diagnostics
- **THEN** the system SHALL only perform read-only diagnostics
- **AND THEN** it SHALL NOT change K3s, Tailscale, firewall, routing, or node configuration
