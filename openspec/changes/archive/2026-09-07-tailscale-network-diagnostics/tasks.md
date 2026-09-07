## 1. Backend diagnostics

- [x] 1.1 Add failing unit tests for K3s network-mode and `tailscale ping` output parsing, including secret and endpoint redaction.
- [x] 1.2 Add fixed, bounded SSH collection for active K3s unit state, K3s VPN marker presence, and structured Tailscale local state.
- [x] 1.3 Add fixed, bounded registered-peer path probes with limited concurrency and partial-result handling.
- [x] 1.4 Add the authenticated read-only server network-diagnostics API and audit entry.
- [x] 1.5 Run `gofmt` and focused Go tests; submit security scan records for changed business files.

## 2. Server page

- [x] 2.1 Add failing frontend tests for refresh, mixed node modes, direct/DERP/unreachable paths, and partial failures.
- [x] 2.2 Add the network-diagnostics server view using the existing navigation, table, badge, and icon-button patterns.
- [x] 2.3 Add loading, empty, unavailable, and error states without introducing arbitrary diagnostic inputs.
- [x] 2.4 Run focused frontend tests and security scan records for changed business files.

## 3. Verification

- [x] 3.1 Run `go test ./...` and `go build ./...`.
- [x] 3.2 Run `PATH=/opt/homebrew/bin:$PATH npm --prefix web test` and `PATH=/opt/homebrew/bin:$PATH npm --prefix web run build`.
- [x] 3.3 Run `openspec validate tailscale-network-diagnostics --strict` and `git diff --check`.
- [x] 3.4 Present all validation results for approval before archiving the OpenSpec change.
