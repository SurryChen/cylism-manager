## MODIFIED Requirements

### Requirement: Import a host directory into a managed PVC

The platform SHALL import an approved host directory into a platform-managed local PVC after creating a local backup and stopping associated writers.

#### Scenario: Import from the PVC node

- **WHEN** the source directory and local PVC are on the same node
- **THEN** the platform SHALL create a backup, copy the directory into the PVC, verify the result, and restore the selected application workload

#### Scenario: Import from another node

- **WHEN** the source directory is on a different reachable cluster node
- **THEN** the platform SHALL stream the archive over the private network to a receiver running on the PVC node
- **AND THEN** it SHALL not mount the local PVC on the source node

### Requirement: Retain and delete local migration backups

The platform SHALL retain a local archive and verification metadata for each completed import until a user explicitly deletes it.

#### Scenario: Delete a verified backup

- **WHEN** a user selects a verified migration backup for deletion
- **THEN** the platform SHALL remove only that archive and record its deletion

#### Scenario: Protect an unverified backup

- **WHEN** a migration has not completed verification
- **THEN** the platform SHALL reject backup deletion
