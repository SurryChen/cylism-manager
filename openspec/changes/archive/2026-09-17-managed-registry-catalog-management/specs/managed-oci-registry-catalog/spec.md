## ADDED Requirements

### Requirement: Browse a managed OCI Registry catalog

The system SHALL provide authenticated, paginated access to repositories and their tags in the configured platform-managed OCI Registry. The server SHALL use the stored managed Registry credential and SHALL return only safe catalog metadata, including repository name, tag, digest, media type, platform information when available, and a complete pull reference.

#### Scenario: List repositories

- **WHEN** an authenticated user requests a page of repositories for a ready managed Registry
- **THEN** the system returns the requested repository page, an opaque continuation value when additional entries exist, and no Registry credential or authorization material

#### Scenario: Inspect repository tags

- **WHEN** an authenticated user opens a repository in the managed Registry
- **THEN** the system returns its paginated Tags with their resolved manifest metadata and pull references

#### Scenario: Registry is unavailable

- **WHEN** the Registry endpoint cannot be reached or authenticated while reading its catalog
- **THEN** the system reports a catalog-read failure and MUST NOT represent the failure as an empty repository list

### Requirement: Manage catalog context in the self-hosted Registry page

The self-hosted Registry page SHALL use a primary header with `概览` and `镜像` tabs. The overview SHALL retain Registry health and configuration actions. The image tab SHALL offer repository search, refresh, paginated repository and Tag views, pull-reference copying, and clear empty, loading, and failure states.

#### Scenario: Open the image tab

- **WHEN** a user selects the `镜像` tab for a configured Registry
- **THEN** the page requests and displays the Registry catalog without reloading the configuration overview

#### Scenario: Empty Registry

- **WHEN** the catalog request succeeds and the Registry contains no repositories
- **THEN** the page displays an empty image state that distinguishes it from Registry connectivity failure

### Requirement: Preflight destructive catalog operations

Before deleting a managed Registry Tag, manifest, or repository, the system SHALL resolve the affected manifest digest, identify every Tag in that repository that references the digest, and check persistent platform Release and deployment-template references. The preflight result SHALL include safe impact metadata and SHALL not expose Registry credentials.

#### Scenario: Tag resolves to a shared digest

- **WHEN** a user requests deletion preflight for a Tag whose digest is referenced by multiple Tags in the same repository
- **THEN** the system returns the complete affected Tag set and requires confirmation against that digest rather than treating the selected Tag as independently deletable

#### Scenario: Platform reference blocks deletion

- **WHEN** a target repository, Tag, or manifest digest is referenced by a persisted Release or deployment template
- **THEN** the system rejects deletion with a conflict response that identifies the protected references

### Requirement: Delete unreferenced managed Registry content safely

The system SHALL delete a Tag or repository only after an explicit confirmation request matches the server-preflighted target and digest impact. Repository deletion SHALL delete each eligible manifest digest only after the entire repository passes reference validation. The system SHALL audit successful, rejected, and failed destructive catalog operations.

#### Scenario: Confirm deletion of an unreferenced Tag

- **WHEN** a user confirms deletion using the preflighted repository, selected Tag, resolved digest, and affected Tag set
- **THEN** the system deletes the digest from the managed Registry, records an audit event, and returns a result that identifies the removed Tags

#### Scenario: Confirm deletion of an unreferenced repository

- **WHEN** a user confirms deletion of a repository whose manifests have no protected platform references
- **THEN** the system removes its eligible manifest digests, records one auditable repository deletion result, and reports any upstream failure without exposing credentials

#### Scenario: Catalog changes after preflight

- **WHEN** the remote manifest digest or affected Tag set differs from the submitted preflight confirmation
- **THEN** the system rejects deletion without changing the Registry and instructs the client to refresh the preflight
