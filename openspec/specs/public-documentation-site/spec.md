# public-documentation-site Specification

## Purpose
TBD - created by archiving change publish-github-pages-documentation. Update Purpose after archive.
## Requirements
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

### Requirement: VitePress build

公开文档站 MUST be buildable with the repository's documented Node command without requiring Python or MkDocs.

#### Scenario: Strict production build

- **WHEN** a contributor runs the documented documentation build command
- **THEN** VitePress produces `docs/.vitepress/dist` and exits successfully
- **AND** the build does not include excluded archive, design, historical deployment, or screenshot-inventory pages

### Requirement: Stable public navigation

The documentation site MUST expose product-domain navigation for 概览、应用交付、制品与供应链、资源与平台、可观测与运维、治理与系统和自动化助手. It MUST retain existing 快速开始、安装部署、配置参考、安全和开发与贡献 pages at their current public URLs as auxiliary documentation.

#### Scenario: Open a product-domain page

- **WHEN** a visitor opens a page under `supply-chain/`
- **THEN** navigation identifies 制品与供应链 as the current product domain
- **AND THEN** the sidebar exposes 自托管制品库、Registry Proxy、节点镜像源 and Helm Chart 仓库

#### Scenario: Follow an existing installation link

- **WHEN** a visitor opens `/installation/install-helm`
- **THEN** the Helm installation page remains reachable at that URL
- **AND THEN** the page provides a route back to the product documentation home

### Requirement: Responsive product navigation

The documentation theme MUST provide an accessible responsive navigation matching the product brand.

#### Scenario: Mobile drawer

- **WHEN** a visitor opens the navigation below the mobile breakpoint
- **THEN** a blue branded header, repository information area, white menu rows, active item state, nested-item indicators, and a dimmed overlay are visible
- **AND** the drawer can be opened and closed by keyboard and pointer without trapping the page permanently

#### Scenario: Desktop homepage

- **WHEN** a visitor opens the homepage on a wide viewport
- **THEN** the homepage uses a readable single-column content layout without duplicate left and right navigation rails

### Requirement: GitHub Pages deployment

The documentation workflow MUST build and publish the VitePress artifact on the supported branches using the configured Pages base path.

#### Scenario: Main branch publish

- **WHEN** the documentation workflow runs for `main`
- **THEN** it builds with the repository's Node lockfile and uploads `docs/.vitepress/dist` for deployment
- **AND** links to assets and pages resolve under the GitHub Pages project path

### Requirement: Documentation quality checks

The repository MUST retain automated checks for documentation structure and public-content safety. The check MUST validate required product-domain files, navigation entries, screenshot placeholder asset-path conventions, excluded paths and successful VitePress configuration.

#### Scenario: Product documentation regression check

- **WHEN** the documentation check script runs
- **THEN** it fails if an approved product-domain page, navigation entry or placeholder contract is missing
- **AND THEN** it continues to reject excluded internal material and unsafe public content

### Requirement: Product-domain documentation coverage

The documentation site MUST provide Chinese operator documentation for every page in the approved seven-domain directory. Each product page MUST state its purpose and boundary, UI entry point, prerequisites, key operations, operational risks and related pages. Composite pages MUST cover their listed sub-capabilities as headings rather than inventing standalone pages.

#### Scenario: Read Kubernetes resource guidance

- **WHEN** a visitor opens `platform/kubernetes-resources`
- **THEN** the page documents 工作负载、服务、配置 and Pod 终端 as separate sections
- **AND THEN** it distinguishes resource inspection from interactive Pod terminal operations

#### Scenario: Read storage guidance

- **WHEN** a visitor opens `platform/storage`
- **THEN** the page documents PVC, directory import, backup and migration workflows
- **AND THEN** it explains data replacement and source-volume cleanup risks before destructive actions

### Requirement: Screenshot placeholders

The documentation theme MUST provide a visible, accessible `ScreenshotPlaceholder` component for planned product screenshots. A placeholder MUST identify the intended UI and an `assets/screenshots/` filename without depicting fabricated product data.

#### Scenario: View a page awaiting a screenshot

- **WHEN** a visitor opens a product page with an unreplaced screenshot placeholder
- **THEN** the page displays the intended screenshot title and description
- **AND THEN** it identifies the suggested screenshot asset path as a placeholder rather than an actual product screenshot

