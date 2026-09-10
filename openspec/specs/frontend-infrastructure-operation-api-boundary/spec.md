# frontend-infrastructure-operation-api-boundary Specification

## Purpose
TBD - created by archiving change frontend-infrastructure-operation-api-boundary. Update Purpose after archive.
## Requirements
### Requirement: Infrastructure operation endpoint ownership is explicit

Servers, cluster nodes, and persistent-volume pages SHALL call named functions from `servers.js`, `cluster.js`, or `storage.js` for reads and mutations rather than constructing infrastructure REST paths in the views.

#### Scenario: An operator manages a registered server

- **WHEN** an operator creates, updates, probes, imports, unbinds, or removes a server
- **THEN** `Servers.vue` SHALL call a named server-domain API function with request options separate from its business payload

#### Scenario: An operator performs node maintenance

- **WHEN** an operator inspects or executes drain, force-drain, rejoin, label, or removal actions
- **THEN** `Cluster.vue` SHALL call a named cluster-domain API function that preserves the existing request contract

#### Scenario: An operator manages persistent data

- **WHEN** an operator creates, migrates, backs up, restores, imports, or deletes a PVC or its import backup
- **THEN** `PersistentVolumes.vue` SHALL call a named storage-domain API function that preserves the existing request contract

### Requirement: Target-dependent infrastructure reads are current and cancellable

Node operation and PVC detail reads that depend on the active target or component lifetime SHALL forward an `AbortSignal` and SHALL cancel or ignore superseded requests.

#### Scenario: An operator changes the selected target during a pending read

- **WHEN** a second node or PVC target is selected before the first target's plan, check, labels, backups, or imports request completes
- **THEN** the earlier result SHALL NOT overwrite data for the current target or its loading state

#### Scenario: An operation surface closes during a pending read

- **WHEN** a node or PVC operation modal closes, or its page unmounts, while a managed read is pending
- **THEN** the request SHALL be cancelled or ignored and SHALL NOT display an abort error or update the closed surface

### Requirement: Infrastructure operation failures are local and non-destructive

A failed server, node, or persistent-volume operation SHALL preserve unrelated loaded data and keep its form, confirmation, or workflow surface available when an operator can correct or retry the action.

#### Scenario: A server mutation fails

- **WHEN** creating, updating, deleting, importing, or unbinding a server fails
- **THEN** the relevant server operation surface SHALL show the failure and the server list SHALL remain available

#### Scenario: A persistent-volume workflow mutation fails

- **WHEN** a PVC migration, backup, restore, import, or deletion action fails
- **THEN** the relevant workflow surface SHALL remain available with an actionable error and previously loaded PVC data SHALL remain visible

