## 1. Persistent configuration and validation

- [x] 1.1 Extend the proxy model and store with identity, Registry, upstream URL and resource name fields.
- [x] 1.2 Validate Registry/upstream pairing, private/Tailscale endpoint, NodePort, cache limit and cleanup interval.
- [x] 1.3 Reject NodePort conflicts with another managed proxy instance.

## 2. Kubernetes proxy lifecycle

- [x] 2.1 Render independent Deployment and NodePort Service resources for each proxy instance.
- [x] 2.2 Set each Deployment's pull-through upstream from the configured Registry.
- [x] 2.3 Preserve legacy Docker Hub resource naming and clear cache by replacing only the selected proxy Pod.
- [x] 2.4 Add an explicit, interruptible migration from the legacy Docker Hub resource name to the per-instance naming scheme.

## 3. API and UI

- [x] 3.1 Add list, create, update and per-instance cleanup REST endpoints while retaining the legacy Docker Hub endpoints.
- [x] 3.2 Replace the single Docker Hub panel with multiple Registry Proxy entries and per-instance configuration.
- [x] 3.3 Add handler and UI coverage for independent `registry.k8s.io` proxy creation.

## 4. Verification

- [x] 4.1 Run focused Go and frontend tests.
- [x] 4.2 Run full Go tests, build, frontend tests and frontend build.
- [x] 4.3 Run strict OpenSpec validation and `git diff --check`.
- [ ] 4.4 Submit sec-code security scans for modified business code files.
