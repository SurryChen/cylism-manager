## 1. Startup template configuration

- [x] 1.1 Add failing backend tests for command/args validation, template serialization and Deployment rendering.
- [x] 1.2 Implement bounded command/args validation and preserve sanitized template snapshots.
- [x] 1.3 Add template editor controls and form serialization for line-based command and args input.
- [x] 1.4 Add frontend tests for create/edit serialization and image-default behavior.

## 2. Release Pod association and runtime diagnostics

- [x] 2.1 Add failing renderer tests proving release labels are applied to Pod templates without changing selectors.
- [x] 2.2 Implement Pod runtime query DTOs and diagnostic extraction with credential redaction.
- [x] 2.3 Extend the Release details handler with application ownership validation and runtime response data.
- [x] 2.4 Add backend tests for Ready, CrashLoopBackOff, normal-exit restart, scheduling failure and legacy Release behavior.

## 3. Release details UI

- [x] 3.1 Add a current Pod runtime panel with state badges, node, restarts and diagnostics.
- [x] 3.2 Keep Release details polling active for live runtime data after terminal orchestration states.
- [x] 3.3 Add UI tests for healthy, unhealthy and legacy-untracked runtime responses.

## 4. Verification

- [x] 4.1 Run `gofmt` and focused Go tests after each backend task, including all callers affected by API changes.
- [x] 4.2 Run `go test ./...` and `go build ./...`.
- [x] 4.3 Run `npm --prefix web test -- --run` and `npm --prefix web run build`.
- [x] 4.4 Submit security scans for every modified business code file and run `git diff --check`.
