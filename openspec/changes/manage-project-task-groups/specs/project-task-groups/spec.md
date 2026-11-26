## ADDED Requirements

### Requirement: Project-scoped task groups
The system SHALL persist groups per product workspace, enforce unique names within that workspace, validate names, and restrict access to the workspace owner.

#### Scenario: Create and rename a group
- **WHEN** the owner creates a group and renames it
- **THEN** the group retains its ID and existing task assignments
- **AND** duplicate names within the workspace are rejected

#### Scenario: Assign a task
- **WHEN** a task is assigned to a group
- **THEN** the group must belong to that task's workspace
- **AND** requests that omit group_id preserve the current assignment

### Requirement: Preserve tasks when deleting groups
Deleting a group SHALL move all its tasks to the ungrouped column and increment their revisions in the same transaction.

#### Scenario: Delete a populated group
- **WHEN** the owner confirms deletion
- **THEN** task contents and version associations are retained
- **AND** stale task edits are rejected

#### Scenario: Reassignment fails
- **WHEN** a task update fails during group deletion
- **THEN** the group and original task assignments remain stored

### Requirement: Group UI capability and compatibility
The toolbar SHALL expose group creation in both layouts and indicate when a backend upgrade is required.

#### Scenario: Older backend
- **WHEN** lifecycle options omit task_groups
- **THEN** the new-group action remains visible and disabled with an upgrade explanation

#### Scenario: Backend upgrade
- **WHEN** the updated backend migrates a legacy database
- **THEN** existing tasks remain intact in the ungrouped column
- **AND** owners can create groups without recreating their workspaces
