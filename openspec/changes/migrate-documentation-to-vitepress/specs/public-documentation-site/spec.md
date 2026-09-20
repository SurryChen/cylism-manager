## ADDED Requirements

### Requirement: VitePress build

公开文档站 MUST be buildable with the repository's documented Node command without requiring Python or MkDocs.

#### Scenario: Strict production build

- **WHEN** a contributor runs the documented documentation build command
- **THEN** VitePress produces `docs/.vitepress/dist` and exits successfully
- **AND** the build does not include excluded archive, design, historical deployment, or screenshot-inventory pages

### Requirement: Stable public navigation

The documentation site MUST expose the existing Chinese documentation sections and preserve their page URLs.

#### Scenario: Main navigation

- **WHEN** a visitor opens the documentation site
- **THEN** the navigation includes 首页、快速开始、安装部署、使用指南、运维与排障、安全和开发与贡献
- **AND** installation and operations groups expose their existing child pages

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

The repository MUST retain automated checks for documentation structure and public-content safety.

#### Scenario: Regression check

- **WHEN** the documentation check script runs
- **THEN** it verifies required public pages, navigation entries, excluded paths and absence of internal deployment credentials
- **AND** it fails on missing files or invalid VitePress configuration
