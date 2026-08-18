## MODIFIED Requirements

### Requirement: Application templates project configuration as files

The platform SHALL allow an application template to select or create same-namespace ConfigMap and Opaque Secret keys for read-only file projection without storing source values in the template.

#### Scenario: Select an existing file resource

- **WHEN** a user configures a file projection in an application template
- **THEN** the platform SHALL list only compatible resources in the application's Namespace
- **AND THEN** selecting a resource SHALL expose only its available key names

#### Scenario: Create a file resource from a template

- **WHEN** a user creates a ConfigMap or Opaque Secret from the template editor
- **THEN** the platform SHALL create an independent namespaced resource through the same authorization and audit path as resource management
- **AND THEN** the new resource SHALL be selected for the current file projection
- **AND THEN** Secret values SHALL not be returned in the template response

#### Scenario: Select a TLS certificate for file projection

- **WHEN** a user selects a Ready managed certificate from an application template
- **THEN** the platform SHALL add read-only projections for the certificate TLS Secret keys `tls.crt` and `tls.key`
- **AND THEN** the platform SHALL not create an HTTP Ingress solely because the certificate was selected

#### Scenario: Publish with a pending TLS certificate

- **WHEN** a template references a certificate that is not Ready or a TLS Secret missing either required key
- **THEN** the platform SHALL block publication
- **AND THEN** the response SHALL identify the unresolved certificate or Secret dependency

#### Scenario: Block deletion of referenced resources

- **WHEN** a ConfigMap, Opaque Secret, managed domain, or Certificate is referenced by a saved template or effective release snapshot
- **THEN** the platform SHALL reject deletion
- **AND THEN** the response SHALL identify the referencing application and template or release
