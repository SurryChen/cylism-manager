## 1. Delivery Registry Proxy workspace

- [x] 1.1 Add failing view tests for delivery-level `制品库` / `Registry Proxy` workspace selection, direct Proxy URL selection, Proxy list rendering, and preserved lifecycle controls.
- [x] 1.2 Move Registry Proxy API functions to a delivery-owned API module with focused API tests; preserve endpoint paths and request payloads.
- [x] 1.3 Refactor the delivery registry view to render the Registry Proxy workspace and retain OCI registry overview/catalog behavior.
- [x] 1.4 Flatten the self-hosted OCI workspace so status and image catalog share one page, with detailed configuration retained in its modal.

## 2. Focus the node mirror view

- [x] 2.1 Add failing view tests that assert no Proxy fetch or Proxy controls on the node mirrors page and verify the explicit full-configuration application copy.
- [x] 2.2 Remove Proxy state, modal flows, lifecycle controls, and Proxy API dependencies from the node mirrors view while retaining rule CRUD, verification, polling, and selected-node application.
- [x] 2.3 Add a contextual delivery-center link and optional same-Registry Proxy endpoint reference that does not couple Proxy lifecycle to node configuration.

## 3. Verification

- [x] 3.1 Run focused frontend API and view tests after each task group.
- [x] 3.2 Run full frontend tests and `npm run build`.
- [x] 3.3 Run `go test ./...`, `go build ./...`, `git diff --check`, and `openspec validate relocate-registry-proxy-management --strict`.
- [ ] 3.4 Present verification results and obtain approval before archiving.
