## 1. Startup template configuration

- [ ] 1.1 Add failing backend tests for command/args validation, template serialization and Deployment rendering.
- [ ] 1.2 Implement bounded command/args validation and preserve sanitized template snapshots.
- [ ] 1.3 Add template editor controls and form serialization for line-based command and args input.
- [ ] 1.4 Add frontend tests for create/edit serialization and image-default behavior.

## 2. Release Pod association and runtime diagnostics

- [ ] 2.1 Add failing renderer tests proving release labels are applied to Pod templates without changing selectors.
- [ ] 2.2 Implement Pod runtime query DTOs and diagnostic extraction with credential redaction.
- [ ] 2.3 Extend the Release details handler with application ownership validation and runtime response data.
- [ ] 2.4 Add backend tests for Ready, CrashLoopBackOff, normal-exit restart, scheduling failure and legacy Release behavior.

## 3. Release details UI

- [ ] 3.1 Add a current Pod runtime panel with state badges, node, restarts and diagnostics.
- [ ] 3.2 Keep Release details polling active for live runtime data after terminal orchestration states.
- [ ] 3.3 Add UI tests for healthy, unhealthy and legacy-untracked runtime responses.

## 4. Verification

- [ ] 4.1 Run `gofmt` and focused Go tests after each backend task, including all callers affected by API changes.
- [ ] 4.2 Run `go test ./...` and `go build ./...`.
- [ ] 4.3 Run `npm --prefix web test -- --run` and `npm --prefix web run build`.
- [ ] 4.4 Submit security scans for every modified business code file and run `git diff --check`.
