## 1. Release Model and Rendering

- [x] 1.0 Add multi-port Service test cases, then evolve the compatible Service model, validation and Kubernetes renderer.
- [x] 1.1 Add failing tests for TCP defaults, UDP container/Service ports, NodePort, LoadBalancer and invalid field combinations.
- [x] 1.2 Extend `ReleaseSpec` Service definitions with backward-compatible defaults and validation.
- [x] 1.3 Render Service type/protocol/traffic policy and remove managed HTTP Ingress when a UDP release is applied.
- [x] 1.4 Add failing tests for ConfigMap and Secret key file projections, invalid paths and missing references.
- [x] 1.5 Implement read-only ConfigMap/Secret file mounts and preflight checks.

## 2. API and Application Lifecycle

- [x] 2.1 Preserve new template fields through create, edit, release snapshot, retry and rollback.
- [ ] 2.2 Extend endpoint APIs and synchronization to distinguish HTTP Ingress from Service exposure while preserving legacy endpoints.
- [ ] 2.3 Add Service status reporting for externally allocated addresses and ports.
- [ ] 2.4 Add backend tests for authorization, validation and backward compatibility.

## 3. Template Editor

- [x] 3.0 Replace the single Service port form with generic multi-port management and preserve legacy template editing.
- [x] 3.1 Add generic network and Service controls with defaults matching existing templates.
- [x] 3.2 Add generic ConfigMap/Secret file-mount controls and serialization.
- [ ] 3.3 Adapt endpoint and application summary views for HTTP Ingress versus Service exposure.
- [x] 3.4 Add frontend tests for default HTTP templates, UDP service templates and file-mount serialization.
- [x] 3.5 Reorganize the template editor into navigable deployment sections without changing template payload semantics.
- [x] 3.6 Replace multiline template inputs with compact row editors while preserving payload semantics.

## 4. Verification

- [x] 4.1 Run `gofmt` and focused Go tests after each backend task, including all callers affected by API changes.
- [x] 4.2 Run `go test ./...` and `go build ./...`.
- [x] 4.3 Run `npm --prefix web test -- --run` and `npm --prefix web run build`.
- [ ] 4.4 Submit security scans for every modified business code file and run `git diff --check`.
