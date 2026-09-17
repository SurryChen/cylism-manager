## 1. Revised node inspection and restart

- [x] 1.1 Add failing Registry service tests for fixed-path inspection, authentication eligibility, YAML parsing, endpoint sanitization, credential redaction, and matching/missing/drifted classification.
- [x] 1.2 Implement the bounded concurrent, read-only Registry configuration inspector and safe comparison DTOs without persisting configuration content.
- [x] 1.3 Add failing delivery Handler and route tests for all-node inspection, partial node failures, safe response data, and audit metadata without configuration content.
- [x] 1.4 Expose the inspection endpoint through bootstrap and routes, preserving current mirror CRUD, verification, and application endpoints.
- [x] 1.5 Add failing tests and implement constrained single-node inspection through the existing endpoint, preserving all-node compatibility.
- [x] 1.6 Add failing tests and implement confirmed fixed-unit K3s service restart with SSH eligibility checks, safe output, audit, handler, route and bootstrap wiring.
- [x] 1.7 Run focused Go service, handler, route, and bootstrap tests.

## 2. Revised node mirror workspace

- [x] 2.1 Add failing frontend API and view tests for the node configuration dialog, single-node inspection, restart confirmation, records dialog, list layout and non-underlined Proxy link.
- [x] 2.2 Rebuild the page around the existing compact single-row list pattern; keep CRUD, verification, polling and selected-node application actions.
- [x] 2.3 Add the node configuration and application-record dialogs; render only safe inspection response data and keep detailed apply output out of the list.
- [x] 2.4 Run focused frontend API and node mirror view tests.

## 3. Verification

- [x] 3.1 Run `go test ./...` and `go build ./...`.
- [x] 3.2 Run the complete frontend test suite and `npm run build`.
- [x] 3.3 Run `git diff --check` and `openspec validate improve-node-registry-mirror-workspace --strict`.
- [ ] 3.4 Present all verification results and obtain approval before archiving.
