## Why

Cylism needs to manage external DNS and object-storage resources, but a provider-specific connection model would make every additional cloud vendor require new routes, tables, views, and workflows. Operators need one secure resource-access workspace that can connect supported cloud providers without exposing credentials to browsers.

## What Changes

- Add provider-neutral cloud connections with a provider key, encrypted credential payload, non-sensitive provider configuration, independently enabled capabilities, and capability-specific validation state.
- Add a provider registry that exposes supported connection schemas and creates provider adapters; Alibaba Cloud is the first adapter, not the public contract.
- Add provider-neutral DNS zone/record and object-storage container/object operations, with provider-specific fields represented as optional metadata rather than core API fields.
- Move provider connection, credential, and capability configuration into System Settings, where administrators manage reusable cloud-provider connections.
- Replace the combined resource-access workspace with separate Cloud Services pages for `域名管理` and `对象存储`; each page displays only the resources and actions for its capability.
- Preserve explicit confirmation, redacted errors, audit metadata, and authenticated bounded upload/download streams for all provider mutations.

## Capabilities

### New Capabilities

- `cloud-resource-connection`: Securely configure, validate, and observe a provider-neutral cloud resource connection.
- `cloud-dns-management`: Discover and manage DNS records through an enabled cloud provider adapter.
- `cloud-object-storage-management`: Discover and manage object-storage containers and objects through an enabled cloud provider adapter.

### Modified Capabilities

- `ui-navigation`: Cloud Services gains separate domain-management and object-storage entries; System Settings gains cloud-provider connection configuration.

## Impact

- Backend: provider registry, provider-neutral connection repository/service/handler layers, encrypted SQLite migration, generic API routes, and audit events.
- Frontend: System Settings provider-connection configuration plus focused Domain Management and Object Storage workspaces, generic API modules and tests, plus navigation and route changes.
- Dependencies: Alibaba Cloud DNS and OSS SDKs remain in the Alibaba adapter only; later providers add adapters without changing public APIs.
- Security: credential payloads are encrypted at rest, never serialized in API responses or audit records; all destructive actions require explicit confirmation.
