## REMOVED Requirements

### Requirement: 本机 tailnet 状态读取
**Reason**: The Manager no longer mounts or accesses the control-plane `tailscaled` socket.

**Migration**: Inspect and administer Tailnet state with Tailscale tooling outside Cylism Manager.

### Requirement: 保存可复用的 Tailscale Auth Key
**Reason**: The Manager no longer installs or authenticates Tailscale on managed hosts.

**Migration**: The obsolete `tailscale_auth_key` system setting is deleted during upgrade. Store any required key with the operator's approved secret-management workflow outside Cylism Manager.

### Requirement: tailnet peer 导入预览
**Reason**: Tailnet discovery is no longer a server-registration source.

**Migration**: Create server records from operator-provided reachable addresses and SSH credentials.
