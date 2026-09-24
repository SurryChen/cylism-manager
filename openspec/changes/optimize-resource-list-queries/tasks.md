## 1. Service lightweight inventory

- [x] 1.1 Add a failing backend test for `endpoint_count=false` proving that a Service list does not query EndpointSlice or Endpoints.
- [x] 1.2 Add the optional lightweight Service list path while preserving the existing default detailed response.
- [x] 1.3 Add a failing frontend API test for the lightweight Service query parameter, then implement the named API function.
- [x] 1.4 Update the Service workspace to use the lightweight list and remove the endpoint-count column; retain per-Service endpoint detail loading.
- [x] 1.5 Run targeted Go and Vue tests for the Service change.

## 2. Lightweight configuration inventory

- [x] 2.1 Add failing Configs workspace tests covering usage=false list requests, omitted reference-count display, and on-demand Secret detail loading.
- [x] 2.2 Update ConfigMap and Secret list reads to use the existing lightweight metadata API functions.
- [x] 2.3 Remove list-level reference counts and optimistic delete disabling; preserve server-side deletion conflict handling.
- [x] 2.4 Load ConfigMap and Secret detail data, including references, only after the user opens the detail flow.
- [x] 2.5 Run targeted Vue and Kubernetes handler tests for the configuration change.

## 3. Deferred namespace dependency

- [x] 3.1 Add a failing Configs workspace test proving the initial list load does not request namespace names.
- [x] 3.2 Load namespace options when the create form opens, with a loading state and existing text-input fallback on failure.
- [x] 3.3 Run targeted Configs workspace tests.

## 4. Full verification

- [x] 4.1 Run `go test ./...` and `go build ./...`.
- [x] 4.2 Run `npm --prefix web test -- --run` and `npm --prefix web run build`.
- [ ] 4.3 Validate the OpenSpec change and present the verification results for user review before archiving.
