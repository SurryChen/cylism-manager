## 1. Documentation contract tests

- [x] 1.1 Add failing documentation-site checks for all approved product-domain pages, their VitePress navigation entries and retained auxiliary-page URLs.
- [x] 1.2 Add failing checks for the `ScreenshotPlaceholder` registration and safe `assets/screenshots/` filename convention.

## 2. Documentation primitives and navigation

- [x] 2.1 Register an accessible `ScreenshotPlaceholder` theme component with a restrained visual placeholder state.
- [x] 2.2 Rework VitePress top navigation and per-domain sidebars to expose the approved seven domains while retaining quick start, installation, configuration, security and contribution links.
- [x] 2.3 Update the homepage to describe platform health, pending alerts, recent releases and key operations, with links into the new product domains.

## 3. Product-domain content

- [x] 3.1 Create the application-delivery and supply-chain pages from the approved directory, using existing product behavior and screenshot placeholders where UI context is useful.
- [x] 3.2 Create the resource-and-platform pages, including grouped Kubernetes resources, network/certificates and storage workflows with explicit data-risk guidance.
- [x] 3.3 Create the observability-and-operations, governance-and-system and automation pages; document only capabilities already exposed by the product.
- [x] 3.4 Cross-link related product domains and existing deployment, configuration and security material without moving old public URLs.

## 4. Verification

- [x] 4.1 Run the documentation structure check and VitePress production build.
- [x] 4.2 Inspect the homepage, a long product page and the mobile drawer in a local preview; verify placeholder labels, navigation and links.
- [x] 4.3 Run `git diff --check` and `openspec validate organize-public-documentation-by-domain --strict`; present results and obtain approval before archiving.
