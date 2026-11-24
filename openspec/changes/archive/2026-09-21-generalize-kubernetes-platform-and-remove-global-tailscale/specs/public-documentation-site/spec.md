## MODIFIED Requirements

### Requirement: Installation documentation follows supported deployment paths

The public documentation SHALL describe supported Kubernetes-compatible and K3s deployment paths, with prerequisites, configuration, verification, upgrade and troubleshooting guidance.

#### Scenario: A new operator selects an installation path

- **WHEN** an operator opens the installation section
- **THEN** they SHALL find supported Helm and documented deployment instructions based on checked-in project assets
- **AND THEN** each path SHALL state Kubernetes API, credential, storage, and optional K3s-specific prerequisites without requiring Tailscale
- **AND THEN** the documentation SHALL not present an unsupported installation method as available

### Requirement: Documentation makes privileged deployment boundaries explicit

The public documentation SHALL explain the operational impact of Kubernetes RBAC, SSH credentials, and SQLite persistence without representing host Tailscale socket access as a deployment requirement.

#### Scenario: An operator reviews prerequisites or security guidance

- **WHEN** an operator reads the installation or security documentation
- **THEN** the documentation SHALL state the privileged cluster access and SSH credentials granted to the Manager
- **AND THEN** it SHALL explain which sensitive values must be stored in Kubernetes Secrets rather than repository files
- **AND THEN** it SHALL state that any Tailscale or K3s VPN credentials are managed outside the Manager
