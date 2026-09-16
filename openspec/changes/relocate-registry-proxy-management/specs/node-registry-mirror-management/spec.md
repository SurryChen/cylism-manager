## ADDED Requirements

### Requirement: Manage desired node registry mirror configuration independently

The platform SHALL provide a cluster node registry mirrors view that manages only desired K3s registry mirror rules, endpoint verification, and selected-node application results. Registry Proxy lifecycle controls SHALL not appear in this view.

#### Scenario: Open node mirror configuration

- **WHEN** a user opens the cluster node registry mirrors view
- **THEN** the platform SHALL show configured Registry rules, their endpoints, verification state, and latest selected-node application results
- **AND THEN** it SHALL provide rule create, edit, delete, verification, and selected-node application actions
- **AND THEN** it SHALL not list or configure Registry Proxy workloads

### Requirement: Make the scope of a node mirror application explicit

The platform SHALL explain before applying a node mirror that all enabled mirror rules are rendered as the selected node's effective K3s registry configuration and that the relevant K3s service will restart.

#### Scenario: Apply to selected nodes

- **WHEN** a user opens the selected-node application dialog for a node mirror
- **THEN** the platform SHALL identify the selected nodes
- **AND THEN** it SHALL state that applying writes the complete enabled registry configuration and restarts the relevant K3s service on each selected node

### Requirement: Keep Registry Proxy endpoint use explicit

The platform SHALL allow node mirror endpoints to reference an available managed Registry Proxy endpoint or an external endpoint without creating an implicit lifecycle dependency.

#### Scenario: Configure an endpoint from a managed Proxy

- **WHEN** a user uses a managed Registry Proxy endpoint in a same-Registry mirror rule
- **THEN** the platform SHALL store the endpoint as part of the mirror rule
- **AND THEN** Proxy deployment, edits, and cache operations SHALL not automatically apply the rule to a node
