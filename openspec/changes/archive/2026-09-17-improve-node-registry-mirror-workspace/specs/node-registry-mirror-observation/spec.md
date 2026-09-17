## ADDED Requirements

### Requirement: Inspect actual node Registry configuration safely

The system SHALL allow an authorized user to explicitly inspect the current K3s Registry configuration of a selected managed cluster node or every managed cluster node. The inspection SHALL use only the platform-managed server inventory and a fixed read-only target path `/etc/rancher/k3s/registries.yaml`; it SHALL not accept an arbitrary host, file path or command. It SHALL not write a node configuration or restart a K3s service.

#### Scenario: Inspect all cluster nodes

- **WHEN** an authorized user requests an actual Registry configuration inspection
- **THEN** the system SHALL inspect every server with a cluster role through the configured SSH access path
- **AND THEN** it SHALL return one result for each eligible node with the inspection time and a current configuration state
- **AND THEN** it SHALL not apply a mirror rule or restart a node service

#### Scenario: Inspect one selected cluster node

- **WHEN** an authorized user chooses a managed cluster node and requests its configuration
- **THEN** the system SHALL validate that node against the managed server inventory and cluster role before inspecting it
- **AND THEN** it SHALL return only that node's safe inspection result

#### Scenario: Report a node that cannot be inspected

- **WHEN** a cluster node has no supported SSH key authentication, cannot be reached, lacks the configuration file, or returns invalid YAML
- **THEN** the system SHALL return a node-specific state and safe operator-facing reason
- **AND THEN** it SHALL continue inspecting the remaining cluster nodes

### Requirement: Compare actual configuration against the complete enabled rule set

The system SHALL compare each successfully inspected node against the complete set of currently enabled node Registry mirror rules. It SHALL classify each node as `matching`, `missing` or `drifted`; a missing configuration file SHALL be `missing` when enabled rules exist and `matching` when no enabled rule exists.

#### Scenario: Detect configuration drift

- **WHEN** a node's Registry endpoints, authentication presence, TLS skip-verification setting, or Registry rule set differs from the currently enabled platform rules
- **THEN** the system SHALL classify that node as `drifted`
- **AND THEN** it SHALL return structured missing, changed, or extra Registry identifiers sufficient to explain the classification

#### Scenario: Confirm a matching configuration

- **WHEN** a node's effective Registry mirror and safe configuration markers equal the complete enabled platform rule set
- **THEN** the system SHALL classify that node as `matching`

### Requirement: Keep actual node configuration data safe to display

The system SHALL parse node `registries.yaml` content on the server side and return only a safe structured summary. HTTP responses, audit events, persisted records, and browser-visible diagnostics SHALL not include raw YAML, Registry passwords, Tokens, encrypted credentials, or URL userinfo.

#### Scenario: View a node configuration difference

- **WHEN** a user opens the details for an inspected node
- **THEN** the system SHALL show only Registry names, endpoints without userinfo, authentication-configured indicators, TLS policy markers, and structured differences
- **AND THEN** it SHALL not show credential values or a raw configuration document

### Requirement: Restart a selected node's K3s service safely

The system SHALL allow an authorized user to restart the K3s service on a selected managed cluster node only after explicit confirmation. It SHALL use a fixed command that detects and restarts either `k3s.service` or `k3s-agent.service`, and SHALL not reboot the host, write Registry configuration or accept an arbitrary service unit or command.

#### Scenario: Restart a K3s node service

- **WHEN** a user confirms restarting the K3s service for a managed cluster node with supported SSH key authentication
- **THEN** the system SHALL restart the detected K3s service and verify that it is active
- **AND THEN** it SHALL return a safe result and record an audit event without configuration content or remote command output

#### Scenario: Reject an unsupported node restart

- **WHEN** a user attempts to restart a node that is not a managed cluster node or lacks supported SSH key authentication
- **THEN** the system SHALL reject the request without opening SSH or executing a command

### Requirement: Organize node mirror operation around list and dialogs

The node Registry mirror workspace SHALL present desired mirror rules separately from observed node configuration state. It SHALL provide a concise rule and health summary, compact mirror rule cards, an explicit actual-configuration inspection command, and node-level configuration detail without rendering long per-node application logs in each rule's primary surface.

#### Scenario: Review mirror rules and node state

- **WHEN** a user opens the node Registry mirror workspace with configured rules
- **THEN** the system SHALL show each rule as one compact list row with Registry, endpoints, verification status, enablement and rule actions
- **AND THEN** it SHALL expose observed node configuration through a top-level node configuration dialog after an inspection is requested
- **AND THEN** it SHALL preserve the existing selected-node application flow as the only configuration-writing action

#### Scenario: View an application record without crowding the rule list

- **WHEN** a rule has node application results
- **THEN** the system SHALL expose them through a deliberate record-view action
- **AND THEN** it SHALL not render per-node application details directly in the rule list

#### Scenario: Navigate to Registry Proxy management

- **WHEN** a user selects the Registry Proxy management link from the node Registry mirror workspace
- **THEN** the system SHALL navigate to `#/delivery/registry?tab=registry-proxy`
- **AND THEN** the link SHALL use the application button appearance without an underlined label
