## 1. Request API Foundation

- [x] 1.1 Add failing tests for applications, servers, and monitoring domain read functions forwarding query parameters and AbortSignal.
- [x] 1.2 Create the three flat domain API modules and preserve existing REST paths and response shapes.
- [x] 1.3 Extend `useAsyncResource` tests for scope disposal and retained last successful data after a failed refresh, if required by the page migrations.

## 2. Applications Request Lifecycle

- [x] 2.1 Add Applications view tests covering project/environment changes while an earlier workspace request is pending.
- [x] 2.2 Migrate Applications managed reads to the applications API module and `useAsyncResource`, retaining current section behavior and command flows.
- [x] 2.3 Run the Applications test file and verify no direct managed read paths remain in the view.

## 3. Servers Request Lifecycle

- [x] 3.1 Add Servers view tests covering section changes and rapid server-stat selection while reads are pending.
- [x] 3.2 Migrate server list, resource statistics, network diagnostics, and server statistics reads to the servers API module and managed resources.
- [x] 3.3 Run the Servers test file and verify commands still refresh the relevant resource.

## 4. Monitoring Request Lifecycle

- [x] 4.1 Add Monitoring view tests covering a rapid trend-range or tab change and a local trend failure after a successful status refresh.
- [x] 4.2 Migrate monitoring status, targets, trends, workloads, and query reads to the monitoring API module and managed resources while preserving migration polling.
- [x] 4.3 Run the Monitoring test file and verify polling cleanup still occurs on unmount.

## 5. Verification

- [x] 5.1 Run `npm --prefix web test` and `npm --prefix web run build`.
- [x] 5.2 Run `go test ./...` and `go build ./...`.
- [x] 5.3 Present the completed verification results for review before archiving the change.
