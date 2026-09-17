## ADDED Requirements

### Requirement: Organize self-hosted Registry operation around service health and image catalog work

The self-hosted Registry workspace SHALL separate low-frequency service operation from high-frequency image catalog operation without adding a nested navigation layer. The runtime overview SHALL group Registry address, readiness, persistent storage, refresh, repair, configuration and deletion. The image catalog SHALL expose its title, repository count and search control together, without separator lines that frame the search control as an independent panel.

#### Scenario: Operate a deployed Registry service

- **WHEN** a user opens a deployed self-hosted Registry workspace
- **THEN** the platform SHALL show Registry service actions with the runtime overview
- **AND THEN** the image catalog SHALL begin as a distinct, labelled workspace below the overview

#### Scenario: Browse the initial image catalog without an empty tag panel

- **WHEN** the platform successfully loads a non-empty Registry catalog and no repository is selected
- **THEN** it SHALL select the first returned repository and load its tags
- **AND THEN** it SHALL show that repository as the active catalog selection

#### Scenario: Continue catalog browsing after removing the selected repository

- **WHEN** a user deletes the currently selected repository and another repository remains in the loaded catalog
- **THEN** the platform SHALL select an available remaining repository and load its tags

