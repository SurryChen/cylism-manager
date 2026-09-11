## 1. API Contracts

- [x] 1.1 Add failing contract tests for credential and temporary-token login, then create `api/auth.js` with the named authentication functions.
- [x] 1.2 Add failing contract tests for all remaining Runtime lifecycle, Agent, chat-session, approval, and streaming functions; move their ownership from `api/index.js` to `api/runtimes.js` without changing request behavior.
- [x] 1.3 Add failing contract tests for database record creation, update, and deletion, then extend `api/admin.js` with named functions.
- [x] 1.4 Add a failing contract test for certificate-operation reads, then extend `api/certificates.js` with its named function and request options support.

## 2. Production Migration

- [x] 2.1 Migrate `Login.vue` to `auth.js` while retaining token persistence and existing successful login navigation.
- [x] 2.2 Migrate `RuntimeManagement.vue` and `ChatDrawer.vue` to `runtimes.js`; verify that no production Runtime chat or Agent endpoint import remains in `api/index.js`.
- [x] 2.3 Migrate `DBAdmin.vue` to `admin.js` and preserve its table editor and delete-confirmation behavior.
- [x] 2.4 Migrate `CertificateOperations.vue` to the certificate API module and a route-keyed `useAsyncResource` lifecycle.

## 3. Regression Coverage

- [x] 3.1 Add view tests for successful login and Runtime migration contracts, plus rejected Runtime lifecycle, grant, and approval actions retaining local retry state.
- [x] 3.2 Add DB Admin failure tests that retain editor values or delete target and loaded rows after a rejected mutation.
- [x] 3.3 Add certificate-operation route-change and unmount tests proving stale results and abort errors are not committed.
- [x] 3.4 Run focused frontend tests after each task; then run `npm --prefix web test`, `npm --prefix web run build`, `go test ./...`, `go build ./...`, `openspec validate --all --strict`, and `git diff --check` before requesting archive approval.
