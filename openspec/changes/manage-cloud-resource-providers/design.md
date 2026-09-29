## Context

Cloud Services currently has no provider-neutral external resource management. The in-progress implementation combines provider credentials, DNS, and object storage into one Alibaba Cloud page, even though connection configuration is administrative and the resource workflows are distinct operator tasks.

The browser must never receive credentials. Provider-specific SDKs, authentication schemes, endpoint configuration, resource options, and error shapes must remain behind server-side adapters. Alibaba Cloud is the first supported provider.

## Goals / Non-Goals

**Goals:**

- Define one connection lifecycle and one generic API surface for all supported providers.
- Support independently enabled DNS and object-storage capabilities, including per-capability validation state.
- Keep provider SDKs and credential interpretation isolated behind typed adapters.
- Put reusable provider connection configuration in System Settings, separate from day-to-day resource operations.
- Provide focused Domain Management and Object Storage pages that display only the corresponding capability.
- Preserve safe confirmations, audit events, error redaction, and bounded streams.

**Non-Goals:**

- Do not promise that every provider supports every capability or every provider-specific option.
- Do not normalize provider-only features into required common fields.
- Do not add compute, billing, IAM, VPC, CDN, or a cloud-resource inventory cache.
- Do not introduce cross-provider resource migration or transactional writes.

## Decisions

### 1. Persist generic connections with encrypted credentials and non-sensitive configuration

`CloudConnection` stores display name, provider key, encrypted credential JSON, configuration JSON, enabled capabilities, validation state, timestamps, and ownership metadata. Credential JSON is interpreted only by the selected adapter and is never serialized. Configuration holds safe values such as a default region or endpoint.

This avoids a schema migration for each provider while preserving queryable common fields. Provider-only connection options stay in JSON, validated against the provider schema before persistence.

### 2. Use a provider registry and capability-specific adapter interfaces

The registry resolves a provider key to connection metadata, validation logic, and adapters. `DNSProvider` and `ObjectStorageProvider` are separate optional interfaces. A provider can support one without the other. Alibaba Cloud implements both interfaces using its existing SDKs.

The current single provider interface forces all providers to implement unrelated APIs and hard-codes an Alibaba AccessKey factory signature. Splitting capabilities keeps the service testable and extensible.

### 3. Make public routes and DTOs provider-neutral

Routes use `/api/cloud/connections`; a connection ID resolves its provider internally. The provider catalog is read from `/api/cloud/providers`. Resource endpoints use `dns` and `object-storage`, not provider product names such as OSS. Common DTOs contain stable fields; `metadata` and `options` carry provider-specific safe attributes.

Existing uncommitted Alibaba-specific routes may be renamed directly because there is no compatibility commitment yet.

### 4. Configure provider connections in System Settings

System Settings owns a cloud-provider section. It lists reusable connections across providers and opens a generic form. The chosen provider supplies labels, credential fields, safe configuration fields, supported capabilities, and policy guidance.

This keeps credential management behind an administrative boundary and prevents provider labels from leaking into generic operational pages.

### 5. Split Cloud Services by operational capability

Cloud Services exposes `域名管理` and `对象存储` alongside the existing registry workspace. Domain Management selects a configured connection with DNS enabled and displays zones and records. Object Storage selects a configured connection with object-storage enabled and displays containers and objects. Each page has a dedicated route and does not embed connection creation or credential editing.

The previous combined page mixes unrelated workflows and makes routine DNS and object operations harder to find. Resource pages may direct users to System Settings when no eligible connection exists.

### 6. Keep safety controls in common services

Confirmation gates, error redaction, audit recording, request size limits, and authenticated streaming remain in generic services and handlers. Provider adapters receive validated inputs and return normalized safe errors.

## Migration Plan

1. Replace uncommitted Alibaba-specific models, repository interfaces, route names, API modules, views, and tests with generic names.
2. Add System Settings routes and views for generic provider connection administration.
3. Split the existing resource page into Domain Management and Object Storage routes, preserving focused capability workflows.
4. Create the generic connection schema and persist existing in-progress connection fields as Alibaba credential/config payloads where local development data exists.
5. Move Alibaba SDK implementation into `providers/aliyun`; register it as `aliyun`.
6. Add future providers by registering adapters and schemas, without public route or common-model changes.

## Risks / Trade-offs

- [Over-generalization] -> Keep common contracts limited to DNS and object-storage primitives; use safe metadata/options for provider-only features.
- [Credential leakage] -> Encrypt credential JSON, omit it from responses/audits, and redact adapter errors.
- [Provider divergence] -> Publish provider capabilities and schemas so unsupported actions are disabled before invocation.
- [Migration complexity] -> The current implementation is uncommitted, so direct renames are preferred over compatibility aliases.
