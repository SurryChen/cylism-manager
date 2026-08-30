# Server and Node HTTP Contracts

This inventory freezes the server and node contracts before their HTTP
adapters move to `internal/api/infrastructure`. All routes are protected by
the existing JWT and audit middleware registered on `/api`.

## Server routes

| Method | Path | Request | Success | Error contract |
| --- | --- | --- | --- | --- |
| POST | `/api/servers` | `name`, `host`, SSH fields | created server | `400/40001` invalid body; `409/40901` duplicate or persistence conflict |
| GET | `/api/servers` | none | ordered server list; stale K8s bindings may be cleared | `500/50000` store error |
| GET | `/api/servers/:id` | numeric ID | server | `404/40404` when absent |
| PUT | `/api/servers/:id` | partial server fields | updated server | `400/40001` invalid body; `404/40404` when absent; `500/50000` persistence failure |
| DELETE | `/api/servers/:id` | numeric ID | `操作成功` | `500/50000` persistence failure |
| POST | `/api/servers/:id/unbind` | numeric ID | server with only `cluster_role` and `k8s_node_name` cleared; `已解除集群绑定` | `400/40001` invalid ID; `404/40404` absent; `500/50000` persistence failure |
| POST | `/api/servers/:id/probe` | numeric ID | `reachable`, `latency_ms`, optional `error` | `400/40001` invalid ID; `404/40404` absent |
| POST | `/api/servers/:id/precheck` | numeric ID | `checks`, `all_pass` | `400/40001` invalid ID; `404/40404` absent |
| GET | `/api/servers/:id/stats` | numeric ID | resource statistics | `400/40001` invalid ID; `404/40404` absent |
| GET | `/api/servers/:id/terminal` | WebSocket | SSH terminal frames | closes connection on invalid ID, missing server, or SSH setup failure |
| GET | `/api/servers/resource-stats` | none | per-server resource statistic result | existing wrapper and partial-failure fields retained |
| GET | `/api/servers/network-diagnostics` | none | tailnet diagnostic snapshots and links | existing wrapper and partial-failure fields retained |

## Node routes

| Method | Path | Request | Success | Error contract |
| --- | --- | --- | --- | --- |
| GET | `/api/nodes` | none | K8s node list | `200/50001` K8s query error; K8s-unavailable response unchanged |
| GET | `/api/nodes/:id/labels` | node name | labels and protected keys | `404/40404` node missing; K8s-unavailable response unchanged |
| PATCH | `/api/nodes/:id/labels` | `set`, `remove` | updated labels | `400/40001` invalid body; `400/40003` label validation failure |
| GET | `/api/nodes/:id/drain-plan` | node name | drain plan | `500/50001` K8s failure |
| POST | `/api/nodes/:id/drain` | `delete_empty_dir_data` | drain result and existing message | `400/40001` invalid body; `409/40901` blocked or failed drain with result payload |
| POST | `/api/nodes/:id/force-drain` | `delete_empty_dir_data`, `acknowledge_risk`, `confirm_node_name` | force-drain result | `400/40001` invalid body; `400/40003` missing acknowledgement/exact node confirmation; `409/40901` K8s rejection with result payload |
| POST | `/api/nodes/:id/rejoin` | node name | node info; `节点已重新加入调度` | `409/40901` K8s failure |
| GET | `/api/nodes/:id/removal-check` | node name | removal check | `500/50001` K8s failure |
| DELETE | `/api/nodes/:id` | node name | `移除成功`; matching servers unbound | `409/40901` unsafe K8s removal; `500/50000` store unbind failure after deletion |
| POST | `/api/nodes/:id/add` | registered server ID | existing placeholder result | `404/40404` server missing |
| GET | `/api/nodes/:id/join-progress` | WebSocket | existing progress-frame protocol | existing progress/error frame semantics retained |
| POST | `/api/nodes/:id/preimport` | registered server ID | matched K8s node information | existing `200/50000` or `200/50001` semantic error wrapper retained |
| POST | `/api/nodes/:id/import` | `hostname`, `role` | saved server/node association; `导入成功` | `400/40001` invalid body; `404/40404` server missing |

`JoinProgress` and `Terminal` keep their WebSocket frame handling in the HTTP
adapter. The underlying server lookup, SSH, and cluster operations are moved
to the Service through explicit dependencies; the Service does not own a Gin
context or WebSocket connection.
