## ADDED Requirements

### Requirement: Cloud Services exposes focused resource workspaces

The system SHALL provide separate Domain Management and Object Storage secondary navigation entries in Cloud Services, while retaining the managed Registry entry and its existing routes. Provider credentials and reusable connection configuration SHALL be accessed from System Settings rather than these operational pages.

#### Scenario: Open domain management
- **WHEN** an authenticated user selects Domain Management under Cloud Services
- **THEN** the system navigates to the domain-management route and shows DNS zones and records for an eligible configured connection

#### Scenario: Open object storage
- **WHEN** an authenticated user selects Object Storage under Cloud Services
- **THEN** the system navigates to the object-storage route and shows containers and objects for an eligible configured connection

#### Scenario: Configure cloud connections
- **WHEN** an administrator opens the cloud-provider section in System Settings
- **THEN** the system shows reusable provider connections and credential/configuration controls without mixing resource inventory into the page

#### Scenario: No eligible connection exists
- **WHEN** a user opens Domain Management or Object Storage without a connection that enables its required capability
- **THEN** the page shows an actionable empty state that directs the user to System Settings to configure a cloud connection

#### Scenario: Registry navigation remains available
- **WHEN** an authenticated user views Cloud Services navigation after the new resource entries are added
- **THEN** the Registry entry remains present and continues to route to /cloud-services/registry
