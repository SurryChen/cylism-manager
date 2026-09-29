## 1. Provider-neutral contracts and persistence

- [x] 1.1 Define the generic connection, provider registry, capability contract, and direct-rename migration plan in proposal/design/specs.
- [ ] 1.2 Add failing model/store tests for generic cloud connections, encrypted credential JSON, safe DTO serialization, provider/config validation state, and schema migration; implement the generic model and repository.
- [x] 1.3 Add the provider catalog contract and discovery tests; expose implemented/unimplemented provider status and capability metadata.
- [ ] 1.4 Move Alibaba Cloud DNS and OSS SDK code into an `aliyun` adapter registered through the provider registry; add adapter contract tests without exposing provider details in common services.

## 2. Generic cloud resource APIs

- [ ] 2.1 Add failing service tests for generic connection create/update/delete, capability validation, disabled-capability rejection, and provider resolution; implement the service.
- [ ] 2.2 Add failing DNS service/handler/route tests for provider-neutral zone and record flows, confirmation-gated deletion, safe errors, and audit events; implement generic endpoints.
- [ ] 2.3 Add failing object-storage service/handler/route tests for container/object flows, bounded streams, confirmation checks, and audit events; implement generic endpoints.
- [ ] 2.4 Search all Alibaba-specific public type, route, and API references; replace or remove them so compiler and frontend tests reveal omissions.

## 3. System Settings provider configuration

- [ ] 3.1 Add failing System Settings router/view tests for reusable cloud-provider connection creation, editing, validation, and deletion.
- [x] 3.2 Add frontend API coverage for the provider catalog and generic cloud-connection administration.
- [x] 3.3 Add schema-driven provider selection, named credential/config inputs, automatic adapter capability detection, unimplemented-provider feedback, and safe credential masking to System Settings.

## 4. Focused Cloud Services workspaces

- [ ] 4.1 Add failing router/navigation tests for Domain Management and Object Storage, while retaining the Registry route; implement navigation updates.
- [ ] 4.2 Add failing Vue tests for DNS zone/record views and Object Storage container/object views, including the empty state that directs users to System Settings when no eligible connection exists.
- [ ] 4.3 Split and rename the existing combined resource view and API tests; ensure no provider-specific wording remains in generic operational pages.

## 5. Full verification

- [ ] 5.1 Run gofmt on touched Go files and go test ./....
- [ ] 5.2 Run go build ./..., npm --prefix web test -- --run, and npm --prefix web run build.
- [ ] 5.3 Validate the OpenSpec change and present all verification results for user review before archiving.
