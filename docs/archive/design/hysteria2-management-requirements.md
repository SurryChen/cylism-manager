# Hysteria2 管理平台需求文档

## 1. 文档信息

- 状态：需求分析稿，待确认
- 日期：2026-08-17
- 目标上线平台：`cylism-manager`
- 参考实现：`cylism-hysteria-manager/docker-deploy`

## 2. 结论摘要

Hysteria2 不应继续作为一组宿主机 Shell 脚本和 Docker 容器被平台“远程执行”。推荐将它建模为 `cylism-manager` 的一类受管应用，运行在 K3s/Kubernetes 中，由平台负责配置、用户、客户端配置文件、版本发布、状态和审计。

一期必须补齐 Kubernetes UDP 外部暴露能力。当前应用发布器生成的是 `ClusterIP` Service，公网入口使用 HTTP Ingress；这两者都不能直接完成 Hysteria2 的公网 UDP 监听。推荐优先使用 UDP `LoadBalancer` Service，无法使用时提供 UDP `NodePort` 回退，并在页面显示最终连接地址和端口。

旧方案中的 Nginx、HTTP/HTTPS 反向代理、静态目录映射和独立 ACME 脚本不再作为 Hysteria2 功能的一部分。需要保留的是 Hysteria2 协议本身所需的 TLS 证书；证书由 `cylism-manager` 的 cert-manager/Secret 能力或用户上传的 Secret 提供，而不是由旧脚本单独维护。

## 3. 现状分析

### 3.1 旧 Docker 部署提供的能力

参考目录中的能力可以归纳为：

| 能力 | 旧实现 | 新平台要求 |
| --- | --- | --- |
| Hysteria2 安装、启动、停止、重启、删除、日志、状态 | Docker Shell 脚本 | 应用发布、操作记录、Pod 状态和日志 |
| 服务端配置 | `/etc/hysteria/config.yaml` | 版本化的配置意图，敏感值写入 Secret，配置只读挂载 |
| 用户管理 | `addUser/listUser/deleteUser` 修改 `auth.userpass` | 页面/API 管理用户，密码不回显，变更可审计 |
| Sing-box 客户端配置 | 根据模板生成每用户 JSON 文件 | 模板管理、按用户生成、持久化、下载/挂载 |
| 用户流量统计 | Hysteria2 `/traffic` API | 平台代理读取、按用户展示、失败可诊断 |
| 在线实例统计 | Hysteria2 `/online` API | 实例数展示和刷新时间 |
| 域名和静态文件目录 | Nginx + `/var/www/<domain>/<user>` | 配置文件资源接口或受控文件存储，不依赖 Nginx |
| 证书申请和续期 | acme.sh + DNS API | 复用平台证书能力；不在 Hysteria2 模块内保存 DNS 凭据 |
| 伪装、DNS、带宽、测速、出口 | YAML 模板 | 管理页面中的受校验字段，支持原始配置导入/导出 |

旧代码中的密码、流量 API secret 和 DNS API 凭据不应迁移为默认值。迁移时必须要求轮换，并视历史文件中的凭据为已暴露凭据。

### 3.2 `cylism-manager` 当前能力边界

已具备：

- Project/Environment/Application/Release 和异步发布流程。
- Deployment/StatefulSet、ConfigMap、Secret、PVC 挂载和资源限制。
- 默认 `ClusterIP` Service。
- 标准 HTTP Ingress、Traefik 检测和 cert-manager Certificate 能力。
- Kubernetes Service、Endpoint、Pod 日志和事件查看。
- Server/SSH 纳管和终端能力。

当前不足：

- `ReleaseSpec.Service` 没有 `protocol`、`type`、`node_port`、`external_traffic_policy` 等字段。
- 应用渲染器固定生成单个 TCP 风格 Service；没有 UDP Service 端口声明。
- 公网 Endpoint 只生成 HTTP Ingress，不能承载 QUIC/UDP。
- 现有镜像代理的 NodePort 逻辑固定创建 TCP 5000 端口，不能作为通用 UDP 转发能力。
- 没有 Hysteria2 专用的流量/在线 API 适配、用户模型或客户端配置文件资源模型。

因此，当前平台不是“只支持转发 UDP”，而是当前通用应用发布路径基本面向 HTTP/TCP；Kubernetes 底层可支持 UDP，但平台尚未暴露和编排这项能力。

## 4. 产品目标

管理员可以在一个 Hysteria2 管理页面中完成以下闭环：

1. 创建或导入一个 Hysteria2 服务实例。
2. 选择 Hysteria2 版本、节点、UDP 监听端口和资源配置。
3. 配置协议 TLS、认证、带宽、DNS、出口、伪装和流量统计。
4. 创建、禁用、删除用户，并生成用户专属 Sing-box 配置。
5. 挂载或下载 Sing-box 配置文件，并查看其版本和关联用户。
6. 查看服务状态、日志、用户流量和在线实例数。
7. 对配置和镜像版本发布、校验、回滚，所有敏感操作可审计。

## 5. 一期范围

### 5.1 Hysteria2 实例

- 实例名称、项目、环境和命名空间。
- Hysteria2 镜像/版本，支持固定 tag，发布时记录 digest（若运行时可解析）。
- 副本数一期固定为 1；多副本需要明确 UDP 会话一致性和流量统计语义后再支持。
- 节点选择、CPU/内存请求与限制、健康检查和 Pod 日志。
- 启动、停止、重启、重新发布、回滚和卸载。
- 配置校验失败时阻止发布，不将不完整配置写入运行资源。

### 5.2 服务端配置

至少支持以下结构化字段：

- 监听地址和 UDP 端口，默认端口可配置，禁止与同一节点已有端口冲突。
- 协议 TLS：证书 Secret、私钥 Secret、证书域名和有效期状态。
- 认证类型 `userpass`，用户由独立用户资源管理。
- 上下行带宽限制、测速开关。
- DNS resolver 类型、地址和超时。
- `direct` 等受支持的 outbound；一期不允许通过页面执行任意命令或任意脚本。
- 伪装类型及其参数，明确允许的 URL/Host 范围并做 URL 校验。
- 流量统计 API 的监听范围和随机生成的 secret。API 只允许 Pod 内部访问，平台通过受控方式读取。
- 高级配置的导入/导出。导入必须经过 schema 校验，未知字段应明确提示，不得静默执行任意配置。

非敏感配置可以写入 ConfigMap；证书、认证密码和流量 API secret 必须写入 Secret，并以只读文件方式挂载到容器。

### 5.3 用户管理

- 用户名唯一、格式受限，不能包含路径分隔符或控制字符。
- 创建用户时生成高强度随机密码，也允许管理员输入符合策略的密码。
- 密码只在创建成功响应中显示一次；列表、详情、日志、审计中只显示脱敏状态。
- 支持启用、禁用、删除、重置密码。
- 删除或重置用户后，关联的客户端配置必须失效或重新生成。
- 用户变更采用幂等操作，并在 Hysteria2 发布完成后才标记为生效。
- 旧配置中的明文密码不作为迁移默认值；迁移操作必须显式确认并建议轮换。

### 5.4 Sing-box 配置文件

一期将 Sing-box 配置视为“客户端配置文件资源”，而不是把它当作 Hysteria2 服务端配置的一部分：

- 管理员可以上传/编辑一个经过 JSON schema 校验的模板。
- 模板中的服务器地址、UDP 端口、用户名密码、TLS server name 等字段由平台按实例和用户注入。
- 每个用户生成一个不可覆盖的配置版本，记录生成时间、模板版本和关联用户。
- 支持在页面下载单个配置，以及按实例导出配置包。
- 支持将配置写入平台管理的 PVC 目录，供受管 Sing-box 客户端以只读方式挂载；挂载路径和 PVC 必须由管理员显式指定。
- 默认不通过公网 Nginx 静态目录暴露配置文件。若后续提供临时下载链接，必须带短时效、单用户授权和审计记录。
- 配置生成不得把密码写入普通日志、审计详情或错误消息。

生成模板至少应能保留旧方案中的 `tun`、DNS、route、direct/block 等常用结构，但一期不承诺平台理解每个 Sing-box 字段；未知字段在模板层保留，在注入字段冲突时阻止生成。

### 5.5 运行状态和统计

- 实例状态：Draft、Deploying、Ready、Degraded、Failed、Stopped、Uninstalled。
- 展示 Deployment/Pod/Service 的实际状态、最近事件和最近一次发布操作。
- 代理读取 Hysteria2 `/traffic`，展示单用户和全量上下行流量，并标明采样时间。
- 代理读取 Hysteria2 `/online`，展示单用户和全量在线实例数。
- Hysteria2 API 超时、非 2xx、返回格式错误时，页面显示可诊断错误，不暴露 secret。
- 支持查看最近 N 次发布和配置变更审计。

## 6. UDP 网络和上线要求

### 6.1 推荐暴露方式

Hysteria2 公网入口必须使用 UDP Service，不使用 HTTP Ingress：

1. 首选：`Service type=LoadBalancer`，端口协议为 `UDP`，外部端口与 Hysteria2 监听端口一致。
2. 回退：`Service type=NodePort`，协议为 `UDP`，平台显示分配的 NodePort，并要求用户在云防火墙/安全组放行 UDP。
3. 只有在明确部署 Traefik UDP entrypoint 和 `IngressRouteUDP` 的环境，才允许增加 Traefik UDP 路由模式；一期不把它作为默认路径。

平台需要新增并校验：

- Service 类型：`ClusterIP`、`NodePort`、`LoadBalancer`。
- 每个 Service 端口的 `protocol`：一期支持 `UDP`，保留 `TCP` 扩展位。
- `node_port`、`external_traffic_policy`、固定外部 IP/地址（如运行环境支持）。
- 节点端口和同协议冲突检查。
- 外部可达地址探测或人工确认，生成客户端配置时使用最终可连接地址，而不是 ClusterIP。
- UDP 连通性预检：Service 存在、Endpoint Ready、节点防火墙提示、外部地址和端口信息完整。

说明：Kubernetes Service 支持 UDP，但 `Ingress` API 是 HTTP 语义；不能通过已有域名 Ingress 配置把 Hysteria2 的 QUIC/UDP 流量转发到服务。

### 6.2 TLS 处理边界

Hysteria2 仍需要协议层 TLS 证书，不能因为移除 Nginx/ACME 就删除 TLS 配置。平台应：

- 复用现有 cert-manager 申请证书，生成 Kubernetes TLS Secret；或允许管理员绑定已有 Secret。
- 将证书 Secret 挂载到 Hysteria2 Pod，并在发布前检查 Secret、域名和证书有效期。
- 提供续期状态和到期告警；证书更新后自动触发安全重载/滚动发布。
- 不在 Hysteria2 模块中保留旧 acme.sh、DNS API Key 或 Nginx SSL 目录。

## 7. 建议的数据模型

建议新增以下平台业务模型，敏感值不通过 API 返回：

```text
hysteria_instances
  id, project_id, environment_id, name, namespace, image, version,
  workload_kind, node_name, service_type, udp_port, node_port,
  external_address, config_revision, status, desired_generation,
  observed_generation, created_by, created_at, updated_at

hysteria_instance_configs
  id, instance_id, revision, non_secret_config, secret_name,
  certificate_secret_name, status, created_by, created_at

hysteria_users
  id, instance_id, username, secret_key, enabled, config_revision,
  created_by, created_at, updated_at, deleted_at

hysteria_client_profiles
  id, instance_id, user_id, template_revision, content_ref,
  content_sha256, status, created_at, revoked_at

hysteria_operations
  id, instance_id, operation_type, status, step, detail,
  request_id, created_by, started_at, completed_at
```

`secret_key` 只用于平台内部定位 Secret 中的键，不保存密码明文。配置快照应脱敏；回滚时从 Kubernetes Secret 和配置版本恢复，不从日志恢复。

## 8. 页面和 API 要求

### 8.1 页面

- 实例列表：状态、版本、UDP 地址/端口、在线用户数、最近发布结果。
- 实例详情：概览、服务端配置、用户、客户端配置、流量、在线实例、日志、发布历史。
- 用户操作：新增、禁用、重置密码、删除、生成/下载 Sing-box 配置。
- 配置编辑：结构化表单 + 高级 YAML/JSON 编辑，保存前执行校验和差异预览。
- 发布向导：预检、资源变更预览、发布进度、失败步骤、重试/回滚。
- 网络提示：明确 UDP、防火墙、安全组、LoadBalancer/NodePort 实际地址，不显示一个不可用的 HTTP URL。

### 8.2 API（建议）

```text
GET/POST/PUT/DELETE /api/hysteria/instances
GET                 /api/hysteria/instances/:id
POST                /api/hysteria/instances/:id/deploy
POST                /api/hysteria/instances/:id/restart
POST                /api/hysteria/instances/:id/rollback
GET                 /api/hysteria/instances/:id/operations

GET/POST/PUT/DELETE /api/hysteria/instances/:id/users
POST                /api/hysteria/instances/:id/users/:userID/reset-password
GET                 /api/hysteria/instances/:id/users/:userID/traffic
GET                 /api/hysteria/instances/:id/users/:userID/online

GET/POST/PUT        /api/hysteria/instances/:id/singbox-templates
POST                /api/hysteria/instances/:id/users/:userID/singbox-profile
GET                 /api/hysteria/client-profiles/:id/download
GET                 /api/hysteria/instances/:id/traffic
GET                 /api/hysteria/instances/:id/online
GET                 /api/hysteria/instances/:id/logs
```

所有写操作使用现有 JWT、审计中间件和项目/环境授权边界。长流程接口立即返回 operation/release ID，前端轮询现有操作接口或后续接入 SSE。

## 9. 资源编排要求

一次发布至少管理以下资源：

- Deployment（一期单副本）或后续 StatefulSet。
- ConfigMap：非敏感 Hysteria2 配置和模板元数据。
- Secret：认证密码、流量 API secret、TLS 证书/私钥。
- Service：UDP 端口，按选择创建 ClusterIP/NodePort/LoadBalancer。
- PVC（可选）：Sing-box 配置文件输出目录，按只读挂载要求绑定。
- Certificate（可选）：由 cert-manager 管理协议 TLS 证书。

资源必须带 `app.kubernetes.io/managed-by=cylism-manager` 和 Hysteria2 实例标签。发布前发现同名但非平台托管资源时必须阻断，不得静默覆盖。

## 10. 非目标和暂缓项

- 不再维护旧 Docker/Nginx/acme.sh Shell 管理脚本作为正式控制面。
- 一期不支持通过标准 HTTP Ingress 转发 Hysteria2 UDP。
- 一期不支持多副本 Hysteria2 集群，除非先定义会话亲和、用户流量聚合和证书/配置一致性。
- 一期不提供任意宿主机命令执行、任意 Docker socket 操作或任意 shell 配置片段。
- 一期不承诺解析并可视化 Sing-box 全部配置字段。
- 若坚持把 Hysteria2 直接安装到宿主机而非 K3s，则需另立“主机代理/Agent 安装器”需求，扩展远程文件、systemd、端口、防火墙和回滚能力，不纳入本需求默认方案。

## 11. 分期建议

### Phase 1：可用的单实例 UDP 服务

- UDP Service 的 protocol/type/NodePort/LoadBalancer 模型。
- 单实例 Hysteria2 发布、配置 Secret、协议 TLS Secret 挂载。
- 用户管理和密码轮换。
- Sing-box 模板、按用户生成和下载。
- 状态、日志、流量/在线统计和审计。

### Phase 2：配置和交付闭环

- Sing-box PVC 挂载和版本目录。
- cert-manager 自动续期后的滚动更新。
- 配置差异、发布回滚、外部 UDP 预检。
- 流量趋势和超额告警。

### Phase 3：高级网络与主机模式

- Traefik `IngressRouteUDP` 和专用 UDP entrypoint。
- 多实例/多节点会话策略。
- 宿主机安装模式、systemd 和防火墙编排。
- 多集群和跨节点流量治理。

## 12. 验收标准

1. 管理员创建一个实例后，平台能生成带 `UDP` 协议端口的外部 Service；页面展示可用于客户端的地址和端口。
2. 未配置 UDP Service 或 Endpoint 未 Ready 时，发布不能标记为成功，并给出具体诊断。
3. Hysteria2 Pod 能读取挂载的服务端配置、认证 Secret 和协议 TLS Secret 并正常启动。
4. 新增用户后能生成唯一密码和 Sing-box JSON；下载文件中的地址、端口、用户名、密码和 TLS server name 正确，日志中没有明文密码。
5. 删除/重置用户后，旧用户配置被标记失效，服务端认证配置完成发布后不再接受旧凭据。
6. `/traffic` 和 `/online` 请求失败时，页面展示脱敏错误和采样时间，不暴露 API secret。
7. 证书更新、配置变更和用户变更均有发布记录、操作者和审计事件，并可回滚到上一成功版本。
8. 访问当前普通 HTTP 应用的发布流程不受影响；新增 UDP 能力通过显式协议字段启用。

## 13. 待确认决策

1. 一期是否确定以 K3s/Kubernetes 部署为唯一运行模式，还是必须同时支持宿主机 Docker/systemd。
2. Hysteria2 对外入口默认使用 `LoadBalancer` 还是固定 `NodePort`；不同云厂商的 UDP LoadBalancer 行为需要实机验证。
3. 协议 TLS 证书是否统一由 cert-manager 签发，还是允许仅绑定用户已有 Secret。
4. Sing-box 配置的“挂载”目标是受管 Sing-box Pod/PVC，还是只需要生成并下载文件；两者的数据模型和权限边界不同。
5. 用户密码是否允许平台可逆读取。推荐仅在生成时写入 Kubernetes Secret，页面只提供一次性展示和重置。
6. 是否需要 Hysteria2 的 UDP 443 与现有 Traefik TCP 443 在同一节点共存；需检查节点防火墙、ServiceLB 和云安全组。

## 14. 参考文件

- `cylism-hysteria-manager/docker-deploy/README.md`
- `cylism-hysteria-manager/docker-deploy/app/hysteria2/docker_hysteria2_manager.sh`
- `cylism-hysteria-manager/docker-deploy/app/hysteria2/hysteria2_manager.go`
- `cylism-hysteria-manager/docker-deploy/app/hysteria2/config.yaml`
- `cylism-hysteria-manager/docker-deploy/app/hysteria2/config.json`
- `cylism-manager/internal/application/spec.go`
- `cylism-manager/internal/k8s/service.go`
- `cylism-manager/internal/k8s/ingress_std.go`
- `cylism-manager/docs/archive/design/application-release-center.md`
