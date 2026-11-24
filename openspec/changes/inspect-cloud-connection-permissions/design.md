## Context

The current Alibaba Cloud connection stores encrypted AccessKey credentials server-side. Its validation calls DNS `DescribeDomains` and OSS `ListBuckets`; it does not inspect RAM identity or policy attachments. The UI currently reports only a connection-level validation summary. Operators want to see assigned DNS/OSS policies and future ECS policies without exposing the AccessKey Secret.

## Goals / Non-Goals

**Goals:**

- Identify the caller represented by an Alibaba Cloud connection's AccessKey.
- List policies attached directly to a RAM user and through that user's groups, with policy name, type, and assignment source.
- Keep a missing RAM-read grant or a non-RAM caller distinguishable from an empty policy list.
- Present a compact, on-demand permission view in System Settings without changing connection credentials.

**Non-Goals:**

- Do not compute effective authorization from policy documents, conditions, resource scopes, explicit denies, or service control policies.
- Do not perform DNS/OSS/ECS write probes or claim that a listed policy proves an operation will succeed.
- Do not add ECS management, policy editing, grant creation, or automatic permission escalation.
- Do not persist or expose credentials, raw policy documents, or provider error payloads.

## Decisions

### 1. Inspect on demand through the existing connection boundary

Add an authenticated `GET /api/cloud/connections/:id/permissions` route. The service decrypts credentials only inside the existing provider-resolution path and calls an optional permission-inspection adapter interface. The response contains a timestamp, caller identity, policy assignments, inspection state (`complete`, `partial`, or `unavailable`), and a sanitized reason. It is not persisted. The handler records an audit event without including policy names or credentials.

Alternative considered: add policy fields to the connection list and refresh them during every page load. Rejected because it would make ordinary settings navigation depend on RAM availability and would unnecessarily collect sensitive authorization metadata.

### 2. Report assignments rather than infer effective permissions

For Alibaba Cloud, use STS `GetCallerIdentity`; for a RAM user, resolve the user name from the validated caller identity, then query RAM direct policies, group membership, and each group's policies. Include policy type and whether it is direct or inherited from a named group. Unsupported principal types (for example, account-root or assumed role) show identity with policy inspection unavailable instead of an empty list. Missing RAM-read permission produces an explicit unavailable or partial state; successful empty results alone mean no attached policies were found.

Alternative considered: parse policy JSON and show "DNS full access" / "OSS full access" badges. Rejected because policy names and documents do not establish effective access when conditions, resource scope, explicit deny, or other authorization layers apply. Existing DNS/OSS validation remains a separate operational signal.

### 3. Keep the UI focused and provider-aware

Add a `查看权限` command for Alibaba Cloud connections. It opens a small modal with caller identity, direct and group-inherited policy rows, refresh time, a clear limitations note, and retry/error states. Unsupported providers do not offer this command until their adapters implement inspection. No AccessKey ID or Secret is shown in the result.

Alternative considered: render policy badges in every connection row. Rejected because the results are fetched on demand, potentially numerous, and need visible provenance and failure states.

## Risks / Trade-offs

- [Additional RAM read permission] -> Document the exact API actions used during implementation and show a missing-permission state; do not require broad `AliyunRAMReadOnlyAccess` for normal DNS/OSS operation.
- [Identity mapping] -> Validate the STS identity type and ARN format before deriving a RAM user name; never query an arbitrary user supplied by the browser.
- [Partial API failures] -> Retain successful direct/group results but label them partial; never display partial results as complete.
- [Sensitive metadata] -> Require existing authenticated API access, redact provider errors, omit raw policy documents, and avoid persistent caching.

## Migration Plan

Add the new route and UI action without schema changes. Rollback removes the route/action; stored connections are unchanged. Implement after the in-progress cloud-provider connection change is accepted.

## Open Questions

- Confirm the Alibaba Cloud RAM API authorization/resource-scoping requirements for listing only the caller's direct and inherited policies before publishing the least-privilege policy example.
