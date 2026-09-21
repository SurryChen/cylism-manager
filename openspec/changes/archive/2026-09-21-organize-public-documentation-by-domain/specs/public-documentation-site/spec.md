## MODIFIED Requirements

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

### Requirement: Documentation quality checks

The repository MUST retain automated checks for documentation structure and public-content safety. The check MUST validate required product-domain files, navigation entries, screenshot placeholder asset-path conventions, excluded paths and successful VitePress configuration.

#### Scenario: Product documentation regression check

- **WHEN** the documentation check script runs
- **THEN** it fails if an approved product-domain page, navigation entry or placeholder contract is missing
- **AND THEN** it continues to reject excluded internal material and unsafe public content

## ADDED Requirements

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
