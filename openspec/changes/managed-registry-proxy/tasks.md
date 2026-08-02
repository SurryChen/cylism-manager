## 1. Persistent configuration and validation

- [ ] 1.1 Add failing Store/model tests for one managed proxy configuration and maintenance status persistence.
- [ ] 1.2 Implement configuration validation for cluster node, private/Tailscale endpoint, NodePort, volume limit, cleanup threshold and interval.

## 2. Kubernetes proxy lifecycle

- [ ] 2.1 Add failing renderer tests for the Registry Deployment, `emptyDir` limit, NodePort Service, labels and Docker Hub upstream configuration.
- [ ] 2.2 Implement apply, readiness inspection, deletion and status collection for managed proxy resources.
- [ ] 2.3 Add failing maintenance tests for cache measurement, threshold cleanup, periodic cleanup and inspection failure.
- [ ] 2.4 Implement background maintenance with pod exec measurement and safe Pod replacement.

## 3. API, mirror application and UI

- [ ] 3.1 Add REST handler tests for configuration CRUD, status, deploy, cleanup and selected-node mirror application.
- [ ] 3.2 Implement handlers and integrate the ready proxy endpoint with node registry mirror rendering.
- [ ] 3.3 Add registry mirror UI controls for proxy configuration, maintenance settings, lifecycle progress and status.
- [ ] 3.4 Add frontend tests for configuration validation and maintenance status presentation.

## 4. Verification

- [ ] 4.1 Run focused Go tests and frontend tests after each task.
- [ ] 4.2 Run `go test ./...`, `go build ./...`, `npm --prefix web test -- --run`, and `npm --prefix web run build`.
- [ ] 4.3 Update RBAC only for required missing permissions and verify `git diff --check`.
- [ ] 4.4 Submit sec-code security scans for every modified business code file.
