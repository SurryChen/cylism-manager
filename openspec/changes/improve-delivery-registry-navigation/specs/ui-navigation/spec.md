## ADDED Requirements

### Requirement: Delivery Registry workspaces use routed query Tabs

The system SHALL expose self-hosted Registry and Registry Proxy workspaces through the delivery Registry aggregation path and a routed `tab` query parameter. The self-hosted Registry workspace SHALL use `/delivery/registry?tab=registry`; the Registry Proxy workspace SHALL use `/delivery/registry?tab=registry-proxy`. Tab selection, browser refresh, direct navigation and browser history SHALL reflect the current query value. Requests without a `tab` query SHALL continue to show the self-hosted Registry workspace.

#### Scenario: Switch delivery Registry workspace

- **WHEN** a user selects the Registry Proxy tab from `/delivery/registry?tab=registry`
- **THEN** the system SHALL navigate to `/delivery/registry?tab=registry-proxy`
- **AND THEN** the Registry Proxy workspace SHALL be active

#### Scenario: Return to self-hosted Registry workspace

- **WHEN** a user selects the self-hosted Registry tab from `/delivery/registry?tab=registry-proxy`
- **THEN** the system SHALL navigate to `/delivery/registry?tab=registry`
- **AND THEN** the self-hosted Registry workspace SHALL be active

#### Scenario: Open a Registry Proxy link

- **WHEN** a user opens `/delivery/registry?tab=registry-proxy`
- **THEN** the Registry Proxy workspace SHALL be active
