## Why

Operators can validate a cloud connection, but cannot see which RAM identity an Alibaba Cloud AccessKey belongs to or which policies are attached to that identity. The existing DNS and OSS read probes do not establish the full set of authorized operations, which becomes more important as compute support is added.

## What Changes

- Add an on-demand, read-only permission inspection for an Alibaba Cloud connection in System Settings.
- Show the AccessKey caller identity and RAM policies attached directly to the RAM user and inherited through groups, including the source of each policy.
- Distinguish complete, partial, and unavailable inspection results, especially when RAM read permission is missing or the caller is not a RAM user.
- Keep the existing connection validation separate from the policy list and label the list as assigned policies, not effective permissions.
- Never return AccessKey credentials, raw policy documents, or unredacted provider errors to the browser.

## Capabilities

### New Capabilities

- `cloud-credential-permission-inspection`: On-demand identity and assigned-policy inspection for a cloud connection, with safe partial-failure behavior and a System Settings view.

### Modified Capabilities

None. The existing cloud-resource connection change remains in progress and is a prerequisite for this capability.

## Impact

- Backend: cloud provider adapter, service, authenticated REST route, audit event, and focused tests.
- Frontend: cloud connection API module, System Settings permission view, and focused tests.
- Alibaba Cloud SDK: reuse the existing STS and RAM packages; no credentials or policy data are persisted.
- Depends on `manage-cloud-resource-providers` being present; this change does not archive or alter that in-progress change.
