## ADDED Requirements

### Requirement: Public documentation is published as a static site

The repository SHALL define a MkDocs-based public documentation site sourced from the checked-in `docs/` directory and a GitHub Pages workflow that publishes the site from the default branch without modifying source branches.

#### Scenario: A maintainer changes public documentation on the default branch

- **WHEN** a change to documentation, site configuration, or the Pages workflow is merged to `main`
- **THEN** GitHub Actions SHALL build the static documentation site and deploy its artifact through GitHub Pages
- **AND** the workflow SHALL not push generated files to a repository branch

#### Scenario: A maintainer validates public documentation on the development branch

- **WHEN** a change to documentation, site configuration, or the Pages workflow is pushed to `dev`
- **THEN** GitHub Actions SHALL strictly build the static documentation site
- **AND** the workflow SHALL NOT upload a Pages artifact or deploy the site

#### Scenario: A contributor previews documentation locally

- **WHEN** a contributor follows the checked-in documentation build instructions
- **THEN** the site SHALL build from the repository's `docs/` directory in strict mode
- **AND** invalid internal links or malformed documentation references SHALL fail the build

### Requirement: Installation documentation follows supported deployment paths

The public documentation SHALL describe only the supported K3s deployment script and Helm Chart installation paths, with prerequisites, configuration, verification, upgrade and troubleshooting guidance.

#### Scenario: A new operator selects an installation path

- **WHEN** an operator opens the installation section
- **THEN** they SHALL find separate script and Helm instructions based on checked-in project assets
- **AND** each path SHALL state required Tailscale, Kubernetes, credential and storage prerequisites
- **AND** the documentation SHALL not present an unsupported installation method as available

### Requirement: Documentation makes privileged deployment boundaries explicit

The public documentation SHALL explain the operational impact of Kubernetes RBAC, the host Tailscale socket, SSH credentials and local SQLite persistence, without including real credentials or private deployment data.

#### Scenario: An operator reviews prerequisites or security guidance

- **WHEN** an operator reads the installation or security documentation
- **THEN** the documentation SHALL state the privileged host and cluster access granted to the Manager
- **AND** it SHALL explain which sensitive values must be stored in Kubernetes Secrets rather than repository files

### Requirement: Product screenshots are governed and safely referenced

The documentation SHALL maintain a screenshot inventory with stable filenames, display guidelines and redaction requirements before screenshots are embedded in public pages.

#### Scenario: A maintainer prepares a product screenshot

- **WHEN** a maintainer consults the screenshot inventory
- **THEN** it SHALL specify the target pages, viewport, naming convention and required redactions
- **AND** it SHALL prohibit publishing tokens, private keys, real addresses, host identifiers, usernames, domains and registry endpoints

### Requirement: Public repository guidance supports contributors and security reports

The repository SHALL provide contribution and security reporting guidance suitable for a public project, including local verification expectations and a private vulnerability reporting path.

#### Scenario: A contributor prepares a pull request

- **WHEN** a contributor reads the contribution guide
- **THEN** it SHALL describe the project validation commands and the OpenSpec change process for behavior changes

#### Scenario: A reporter discovers a security vulnerability

- **WHEN** a reporter opens the security policy
- **THEN** it SHALL instruct them not to disclose credentials or exploitable details in a public issue
- **AND** it SHALL provide a private reporting contact or GitHub private advisory path
