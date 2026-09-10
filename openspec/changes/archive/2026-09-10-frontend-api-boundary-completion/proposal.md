## Why

The frontend has completed most of its domain API consolidation, but five production workflows still bypass the domain boundary or keep feature-specific endpoints in the generic transport module. This leaves endpoint contracts scattered across views, makes request lifecycle handling inconsistent, and increases the chance that high-risk operational forms lose useful retry state after a failure.

## What Changes

- Add named authentication functions for password and temporary-token login, while retaining token storage and request transport in the generic API module.
- Complete Runtime API ownership for runtime lifecycle actions, Agent grants and approvals, chat sessions, and streaming chat requests.
- Complete database administration and certificate-operation API ownership for their remaining view-level REST calls.
- Migrate affected views and the Runtime Chat Drawer to named domain functions without changing backend endpoints, authorization, payloads, or visible workflows.
- Make certificate-operation reads cancellable and current for the active certificate route.
- Add API contract and view regression tests for authentication, Runtime, database administration, certificate operations, retry state, and request lifecycle behavior.

## Capabilities

### New Capabilities

- `frontend-api-boundary-completion`: Complete named domain API ownership and request-lifecycle guarantees for the remaining frontend operational workflows.

### Modified Capabilities

- None.

## Impact

- Affects `web/src/api/index.js`, adds `web/src/api/auth.js`, and extends `runtimes.js`, `admin.js`, and `certificates.js` with corresponding tests.
- Affects `Login.vue`, `RuntimeManagement.vue`, `DBAdmin.vue`, `CertificateOperations.vue`, `ChatDrawer.vue`, and their focused tests.
- Does not modify Go backend endpoints, application data, authentication policy, Kubernetes resources, or deployment configuration.
