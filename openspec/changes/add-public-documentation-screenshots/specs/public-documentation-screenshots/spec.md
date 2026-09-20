## ADDED Requirements

### Requirement: Product documentation uses reviewed interface screenshots

The documentation site MUST replace the existing screenshot placeholder in each of the 18 approved product pages with a real PNG screenshot stored under `docs/assets/screenshots/`. Every replacement MUST use the page's existing suggested filename and include meaningful Chinese alternative text in Markdown.

#### Scenario: Open a documented product page

- **WHEN** a visitor opens the documentation homepage or one of the approved product pages
- **THEN** the page displays its real product screenshot instead of a screenshot placeholder
- **AND THEN** the image source resolves to the matching local `assets/screenshots/` PNG file

#### Scenario: Maintain an unapproved future page

- **WHEN** a future product page has no reviewed product screenshot
- **THEN** the documentation theme MAY continue to display `ScreenshotPlaceholder`
- **AND THEN** no fabricated screenshot is displayed

### Requirement: Published screenshots exclude browser and sensitive data

Every published screenshot MUST contain only the Cylism product page area and MUST NOT expose browser chrome, addresses, credentials, Tokens, private keys, complete kubeconfig data, usernames, host or node identifiers, real domains, registry endpoints, customer names, or unredacted error logs.

#### Scenario: Review a screenshot before publication

- **WHEN** a maintainer prepares a product screenshot for `docs/assets/screenshots/`
- **THEN** the maintainer verifies the image is a task-focused page crop without browser chrome or prohibited sensitive values
- **AND THEN** an image that cannot satisfy these constraints remains unpublished

### Requirement: Documentation checks validate screenshot assets and references

The documentation verification script MUST fail when an approved replacement page references a missing screenshot, points outside `assets/screenshots/`, or retains its replaced screenshot placeholder.

#### Scenario: Run the documentation site check with complete screenshots

- **WHEN** all approved screenshot PNG files exist and their Markdown pages reference them
- **THEN** the documentation structure check succeeds

#### Scenario: Remove an approved screenshot asset

- **WHEN** an approved page retains its Markdown image reference but its PNG file is missing
- **THEN** the documentation structure check fails before the documentation site is published
