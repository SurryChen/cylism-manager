## 1. Audit query contract

- [x] 1.1 Add created-from/created-to fields to the audit filter and API parsing.
- [x] 1.2 Apply the date bounds in the repository and test inclusive/exclusive behavior.

## 2. Audit filter experience

- [x] 2.1 Refactor the audit page into a compact primary bar and advanced filter modal.
- [x] 2.2 Align resource/action options with emitted audit events and show active filter chips.
- [x] 2.3 Add frontend tests for modal apply, reset, and date query parameters.
- [x] 2.4 Add and adopt a reusable custom select component for the audit filters.

## 3. Verification

- [x] 3.1 Run focused Go and frontend tests.
- [x] 3.2 Run frontend build, `go build ./...`, diff check, and OpenSpec validation.
- [x] 3.3 Run full `go test ./...`; the current environment completes the suite successfully.
