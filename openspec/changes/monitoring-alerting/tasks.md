## 1. Alerting Resources and Rules

- [x] 1.1 Define alerting configuration, status, default rule rendering, and input validation with unit tests.
- [x] 1.2 Create/reconcile managed Alertmanager Service, PVC, ConfigMaps, Secret, Deployment, vmalert Deployment and least-privilege kube-state-metrics with readiness/status tests.
- [x] 1.3 Add safe uninstall behavior that preserves Alertmanager PVC, notification Secret and node binding configuration, with regression tests.

## 2. Alerting APIs and Notification Relay

- [x] 2.1 Add authenticated alerting installation, status, settings, active-alert, silence and test-notification APIs with handler tests.
- [x] 2.2 Add a bearer-token-protected internal Alertmanager callback that renders and forwards Feishu messages without exposing secrets, with authorization and payload tests.
- [x] 2.3 Register routes and verify unauthenticated callback isolation from user API authentication.

## 3. Alerting Workspace

- [x] 3.1 Add the monitoring Alerting view, loading states, counters, active/recovered alert lists and node/workload navigation with component tests.
- [x] 3.2 Implement silence confirmation and settings drawer for rule configuration and Feishu channel management with request/error-state tests.
- [x] 3.3 Add the alerting installation card and explicit unavailable/healthy states, preserving the shared monitoring page layout.
- [x] 3.4 Replace the settings drawer with a centered high-layer modal and add SMTP email notification configuration.

## 4. Verification

- [x] 4.1 Run focused Go and Vue tests while completing each task, then run `go test ./...`, `go build ./...`, frontend tests and production build.
- [ ] 4.2 Run OpenSpec validation and the required security scan for every modified business-code file.
