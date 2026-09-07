## 1. Baseline and Test Layout

- [x] 1.1 Review the existing Agent endpoint tests and map each test to workload, Registry, observability, or maintenance behavior.
- [x] 1.2 Ensure shared Agent HTTP test setup remains in one test helper file and add focused regression coverage for any endpoint family lacking it.

## 2. Same-Package Handler Organization

- [x] 2.1 Move workload and cluster endpoint methods with their tests into a workload Handler source file; run `go test ./internal/api/agent`.
- [x] 2.2 Move Registry endpoint methods with their tests into a Registry Handler source file; run `go test ./internal/api/agent`.
- [x] 2.3 Move DNS, alert, and monitoring endpoint methods with their tests into an observability Handler source file; run `go test ./internal/api/agent`.
- [x] 2.4 Move maintenance endpoint methods with their tests into a maintenance Handler source file; run `go test ./internal/api/agent`.

## 3. Verification

- [x] 3.1 Confirm public constructors, Bootstrap composition, and `routes_public.go` have no behavioral diff; run the Router snapshot test.
- [x] 3.2 Run `go test ./internal/api/...`, `git diff --check`, and `openspec validate api-agent-file-organization --strict`.
