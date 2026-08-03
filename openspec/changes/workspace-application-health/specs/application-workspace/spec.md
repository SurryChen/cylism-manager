## ADDED Requirements

### Requirement: Workspace shows current service health
The workspace SHALL show each selected-environment application with its current runtime state and active version.

#### Scenario: A previous version remains available after a failed release
- **WHEN** the newest release fails and the latest successful release has ready Pods
- **THEN** the application runtime state remains running while the latest release result remains visible
