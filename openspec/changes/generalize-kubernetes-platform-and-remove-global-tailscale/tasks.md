## 1. Platform identity contract (TDD)

- [x] 1.1 Add failing backend tests for Kubernetes, K3s, and unavailable discovery classification, including sanitized identity responses and K3s-only capability flags.
- [x] 1.2 Add the narrow Kubernetes discovery adapter, cluster identity API, route wiring, and API contract fixtures until the focused tests pass.
- [x] 1.3 Add failing frontend tests for Kubernetes, K3s, and unknown platform identity display and feature gating.
- [x] 1.4 Render the current cluster platform and version in the UI and hide K3s-only actions when the capability is unavailable.

## 2. K3s worker bootstrap without Tailscale (TDD)

- [x] 2.1 Add failing join-workflow tests that assert no Tailscale command, installer, Auth Key lookup, or Tailscale-specific progress step is used.
- [x] 2.2 Remove Tailscale installation and registration from the node join workflow; preserve bounded SSH preflight, K3s agent installation via the configured control-plane address, readiness polling, and sanitized errors.
- [x] 2.3 Require discovered K3s capability in the join route and UI; test that Kubernetes and unknown platforms cannot start a K3s worker join.

## 3. K3s VPN compatibility diagnostics (TDD)

- [x] 3.1 Add failing parser and handler tests for active K3s VPN marker detection, safe provider classification, standard K3s/Kubernetes results, partial SSH failure, and credential redaction.
- [x] 3.2 Replace Tailnet state collection and peer probes with bounded read-only K3s VPN marker collection; execute no Tailscale command unless a future separate capability authorizes one.
- [x] 3.3 Update server diagnostic API fixtures and frontend tests so the K3s VPN status is shown only for active K3s VPN integration and no Tailscale columns appear otherwise.

## 4. Remove platform-level Tailscale (TDD)

- [x] 4.1 Add failing route, bootstrap, and system-config migration tests covering the absence of all `/api/tailscale/*` endpoints and idempotent removal of only `tailscale_auth_key` without secret logging.
- [x] 4.2 Delete the Tailscale handler/service/runtime wiring, routes, route golden entries, Tailscale-specific audit handling, and obsolete tests; implement and test the scoped encrypted configuration cleanup migration.
- [x] 4.3 Remove Tailnet discovery/import and Tailscale metadata from server-management APIs and UI; retain operator-provided management addresses and existing SSH credentials.

## 5. Generic deployment and documentation (TDD)

- [x] 5.1 Add failing chart/static-manifest checks that reject Tailscale CLI installation, `/run/tailscale` mounts, Tailscale values, and hard-coded control-plane host selection while requiring PVC-backed default persistence.
- [x] 5.2 Update Dockerfile, Helm values/templates, static Kubernetes assets, and deployment tests until the rendered deployment is Kubernetes-compatible without host Tailscale access.
- [x] 5.3 Update README and public documentation to describe Kubernetes compatibility, optional K3s VPN diagnostics, direct K3s node joining, PVC prerequisites, and the removed Tailscale APIs.

## 6. Verification

- [x] 6.1 Run `gofmt` and focused backend/frontend/chart/documentation tests after each completed task group.
- [x] 6.2 Run `go test ./...`, `go build ./...`, `npm --prefix web test`, `npm --prefix web run build`, strict documentation checks, `openspec validate generalize-kubernetes-platform-and-remove-global-tailscale --strict`, and `git diff --check`.
- [ ] 6.3 Present the breaking-change migration note and complete verification results for approval before archiving.
