## MODIFIED Requirements

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
