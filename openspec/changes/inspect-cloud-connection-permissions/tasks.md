## 1. Permission inspection contract

- [ ] 1.1 Add failing service tests for supported/unsupported providers, caller identity, direct and inherited policies, empty complete results, partial failures, and credential redaction; define the provider-neutral inspection DTO and optional adapter interface.
- [ ] 1.2 Implement the service contract and Alibaba Cloud STS/RAM inspector using the existing SDK; validate caller identity before RAM queries and sanitize errors.

## 2. Authenticated API

- [ ] 2.1 Add failing handler and route tests for the permission endpoint, authentication, unsupported providers, partial results, and secret-free responses/audit details.
- [ ] 2.2 Add the read-only inspection route and audit event; search existing cloud connection call sites for contract changes.

## 3. System Settings experience

- [ ] 3.1 Add failing frontend API and view tests for the permission command, loading/retry, direct/group provenance, empty/partial/unavailable states, and authorization caveat.
- [ ] 3.2 Implement the on-demand permission modal using existing components and design tokens; keep unsupported providers and credential values out of the view.

## 4. Verification

- [ ] 4.1 Run focused Go tests after each backend task, gofmt, and `go test ./...`.
- [ ] 4.2 Run focused frontend tests after each UI task, the full frontend suite, `go build ./...`, and `npm --prefix web run build`.
- [ ] 4.3 Validate this OpenSpec change and present all verification results for user review before archiving.
