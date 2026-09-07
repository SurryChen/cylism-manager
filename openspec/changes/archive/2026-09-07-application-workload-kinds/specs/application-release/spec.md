## ADDED Requirements

### Requirement: Applications select a workload controller
The platform SHALL render releases as a Deployment or StatefulSet according to the application workload kind.

#### Scenario: Stateful application release
- **WHEN** an application with workload kind `statefulset` is released
- **THEN** the platform creates or updates a StatefulSet with the release Pod template and stable Service selector

### Requirement: Existing workload type changes protect PVC data
The platform SHALL scale down the prior managed controller before creating the replacement controller for a type migration.

#### Scenario: Deployment with an RWO PVC changes to StatefulSet
- **WHEN** the application has a successful single-replica release and changes workload kind
- **THEN** the old Pod is stopped before the StatefulSet reuses the same PVC
