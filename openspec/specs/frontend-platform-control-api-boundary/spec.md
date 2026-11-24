# frontend-platform-control-api-boundary Specification

## Purpose
TBD - created by archiving change frontend-platform-control-api-boundary. Update Purpose after archive.
## Requirements
### Requirement: Platform control endpoint ownership is explicit

System components, system settings and Cluster DNS pages SHALL invoke named functions from their domain API modules for control-plane reads and mutations rather than constructing REST paths in the page.

#### Scenario: An operator changes a system component

- **WHEN** an operator saves component configuration or restores its default configuration
- **THEN** `SystemComponents.vue` SHALL call a named function from `system-components.js` with the existing payload and endpoint contract

#### Scenario: An operator changes platform settings

- **WHEN** an operator configures the platform endpoint, creates or revokes a temporary token, generates a webhook secret, changes an image prefix, releases an image, or rolls back a release
- **THEN** the corresponding system settings subpage SHALL call a named function from `settings.js`

#### Scenario: An operator manages Cluster DNS

- **WHEN** an operator reads, saves, clears, or rolls back a Cluster DNS policy
- **THEN** `ClusterDNS.vue` SHALL call a named function from `cluster-dns.js` that preserves the existing request contract

### Requirement: Platform control reads are current and cancellable

Control-plane reads that can overlap because of refresh, target change, tab change or component disposal SHALL accept request options and SHALL cancel or ignore superseded results. A cancelled or superseded read SHALL NOT be rendered as a user-facing error; a current read that fails for a non-cancellation reason SHALL retain its local error presentation.

#### Scenario: A later Cluster DNS read completes first

- **WHEN** a newer DNS policy read starts before an earlier read settles
- **THEN** the older result SHALL NOT overwrite the newer policy data or loading state

#### Scenario: A platform control page unmounts during a read

- **WHEN** a system component, settings region, or DNS page is removed while its managed read is pending
- **THEN** its request SHALL be cancelled or ignored and SHALL NOT update disposed state or show an abort error

#### Scenario: A newer platform status refresh cancels an earlier refresh

- **WHEN** the 发布与更新 settings region starts a newer `/api/platform/status` read before its previous read settles
- **THEN** the cancelled earlier read SHALL NOT display a status-read failure
- **AND THEN** only the latest successful response SHALL update release status and image-prefix form state

#### Scenario: The current platform status refresh fails

- **WHEN** the active `/api/platform/status` read fails for a reason other than cancellation
- **THEN** the 发布与更新 settings region SHALL display the returned local failure without clearing previously rendered platform status

### Requirement: Platform control failures are local and non-destructive

A failed platform control mutation SHALL retain the relevant form, confirmation or dialog and SHALL preserve successful data from unrelated page regions.

#### Scenario: A system component configuration save fails

- **WHEN** a component configuration mutation is rejected
- **THEN** the configuration modal SHALL remain open, expose the failure locally, and retain the entered values

#### Scenario: A release or DNS action fails

- **WHEN** a platform release, endpoint action, temporary token action, or DNS mutation is rejected
- **THEN** the initiating surface SHALL expose an actionable failure while the last successful release, endpoint, token, or DNS data remains available

