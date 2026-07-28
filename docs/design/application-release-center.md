# Cylism Manager 应用发布中心设计

## 1. 背景与目标

当前平台已具备服务器、K3s 节点、工作负载、Service、ConfigMap、Secret、Ingress/IngressRoute 和 Certificate 的管理能力。它们适合排障和高级资源操作，但一次服务上线仍需要用户在多个页面间手动协调资源，且缺少发布版本、校验、回滚和责任归属。

本设计新增“应用发布中心”，将一次服务上线建模为一个可验证、可审计、可回滚的发布单。它不替代 Kubernetes 原生资源，也不引入新的编排系统；平台仍以 Kubernetes API 作为运行时真实来源。

### 1.1 目标

- 让用户通过一个入口完成镜像、配置、工作负载、Service、证书和路由的上线。
- 将发布过程拆分为可观测步骤，失败时给出可操作的原因和重试入口。
- 保持现有资源页可用：应用发布中心负责日常发布，基础设施页负责资源查看、紧急修复和高级操作。
- 支持公网域名和仅 Tailnet 内访问两种入口模式，避免把 Tailscale 误当作公网入口。
- 为后续 CI/CD、GitOps、监控告警和多环境治理预留稳定边界。

### 1.2 非目标

- V1 不提供源码构建、镜像仓库托管或 Git 仓库 Webhook；支持接入外部 OCI/Docker 镜像仓库并管理其拉取凭据。
- V1 不自动管理外部 DNS 托管商账号；仅记录验证前置条件，并支持用户提供已解析的域名。
- V1 不替代 Helm、Argo CD 或 Flux；高级用户仍可使用原生工具管理非平台托管资源。
- V1 不支持跨集群流量治理、蓝绿/金丝雀发布和复杂四层协议路由。

## 2. 设计原则

1. **Kubernetes 是真实来源。** Deployment、Service、Ingress、Certificate 等运行状态始终从 Kubernetes API 获取；数据库只保存应用元数据、发布意图、审计和关联关系。
2. **以应用而非资源为中心。** 用户管理的是“订单服务生产环境”，而非孤立的 Deployment 或 Ingress。
3. **声明式发布，受控执行。** 每个发布版本生成一组可审阅的目标资源，校验通过后按依赖顺序 apply。
4. **最小默认暴露。** 默认创建 ClusterIP Service；公开访问必须显式选择域名和入口方式。
5. **安全默认值。** Secret 只写入、不回显；资源限制、健康检查、滚动更新和审计为上线必填或明确豁免项。

## 3. 用户与核心概念

| 概念 | 说明 | 运行时真实来源 |
| --- | --- | --- |
| 项目 Project | 一组有共同归属和权限边界的应用 | 平台数据库 |
| 环境 Environment | 项目的 dev、staging、prod 等部署目标 | 平台数据库，映射默认 Namespace |
| 应用 Application | 可独立发布、运维和回滚的服务 | 平台数据库 + K8s 资源标签 |
| 发布 Release | 应用某次不可变镜像版本和配置快照 | 平台数据库 + K8s revision |
| 运行配置 | ConfigMap 和 Secret 的键值及引用 | Kubernetes API |
| 服务入口 Endpoint | ClusterIP Service、域名路由、证书和访问模式 | Kubernetes API + 平台数据库 |
| 发布任务 Operation | 预检、apply、就绪等待、验证等步骤记录 | 平台数据库 / 操作日志 |

应用创建后必须绑定一个 Project 和 Environment。V1 中一个应用对应一个 Namespace 内的主工作负载；同一应用需要多组件时，使用多个应用记录，而非在首次实现中引入复杂的应用图。

## 4. 上线流程

```text
创建应用草稿
  -> 填写镜像、端口、资源、配置、域名
  -> 发布前校验
  -> 创建/更新 ConfigMap 与 Secret
  -> apply Deployment
  -> 等待 Pod Ready
  -> apply ClusterIP Service
  -> 创建 Certificate（可选）
  -> 等待 Certificate Ready（HTTPS 时）
  -> apply Ingress/IngressRoute（可选）
  -> 连通性验证
  -> 标记 Release 成功，可回滚
```

### 4.1 推荐的资源依赖顺序

1. Namespace：不存在时创建，或要求用户选择已有 Namespace。
2. Secret / ConfigMap：先创建，避免 Pod 因引用不存在而无法启动。
3. Deployment：使用不可变镜像 tag 或 digest，配置 `readinessProbe`、`livenessProbe`、resources 和滚动升级策略。
4. Service：默认 `ClusterIP`，selector 必须与 Deployment 的固定标签匹配。
5. Certificate：HTTPS 域名使用 cert-manager 的 Certificate 资源，输出 TLS Secret。
6. Ingress 或 IngressRoute：引用已就绪的 Service 端口和 TLS Secret。

任何步骤失败时，发布状态设为 `failed`，保留已创建资源和完整步骤日志供诊断。V1 不自动删除已成功创建的资源，避免在失败处理时误删用户已有配置；用户可执行“回滚”或“删除应用”。

### 4.2 域名、证书与网络前置条件

| 访问模式 | DNS 要求 | 证书方式 | 入口要求 |
| --- | --- | --- | --- |
| 公网 HTTP/HTTPS | 域名 A/AAAA/CNAME 指向公网入口 | HTTP-01 或 DNS-01 | Traefik 的 80/443 可从公网访问 |
| 公网 HTTPS，80 不可达 | DNS 记录由 DNS 服务商托管 | DNS-01 | 平台或用户具备 DNS API 凭据 |
| Tailnet 私有访问 | 不要求公网 DNS，可使用内部名称 | 可选内部 CA/自签名证书 | 客户端已加入 Tailnet，能访问入口 Tailscale IP |

Tailscale 仅负责节点和授权客户端之间的私有连通性，不能替代公网 DNS 和公网 ACME 验证。选择公网 HTTP-01 时，平台必须提示用户确认：DNS 已解析、入口节点具备公网可达的 80/443、Traefik 已运行。选择 DNS-01 时，DNS API 凭据必须作为 Secret 保存，并限制为对应 DNS Zone 的最小权限。

## 5. 数据模型与资源归属

### 5.1 平台数据库

```text
projects(id, name, description, owner_id)
environments(id, project_id, name, namespace, cluster_id)
applications(id, project_id, environment_id, name, workload_kind, created_by)
application_endpoints(id, application_id, exposure, domain, path, service_port, certificate_ref)
releases(id, application_id, sequence, image, image_digest, desired_spec, status, created_by, started_at, completed_at)
release_operations(id, release_id, step, status, detail, started_at, completed_at)
```

`desired_spec` 保存经过脱敏的发布定义和资源版本引用，不保存 Secret 明文。Secret 值仅在提交时写入 Kubernetes；发布记录只保留 Secret 名称与键名。

### 5.2 Kubernetes 标签与命名

平台创建的资源必须带以下标签，供资源页反向关联与审计：

```yaml
app.kubernetes.io/managed-by: cylism-manager
app.kubernetes.io/part-of: <project>
app.kubernetes.io/name: <application>
cylism.io/environment: <environment>
cylism.io/release: <release-sequence>
```

资源名称使用 `<application>` 为基础：Deployment、Service 和 ConfigMap 同名；Secret 采用 `<application>-secret`；Certificate 采用 `<application>-tls`。域名可多条，但 V1 每个应用仅支持一个 Service。

## 6. 发布定义

发布中心以表单为主，并提供“查看渲染 YAML”用于审阅，不要求用户先手写 YAML。

```yaml
application:
  name: order-api
  namespace: production
image: registry.example.com/order-api@sha256:...
containerPort: 8080
replicas: 2
resources:
  requests: { cpu: 100m, memory: 128Mi }
  limits: { cpu: 500m, memory: 512Mi }
health:
  readinessPath: /healthz
  livenessPath: /healthz
service:
  port: 80
  targetPort: 8080
exposure:
  mode: public
  domain: api.example.com
  path: /
  tls:
    enabled: true
    issuerRef: letsencrypt-prod
```

V1 表单将以下字段设为必填：镜像、容器端口、资源 requests/limits、readinessProbe、Service 端口和 Namespace。工作负载类型默认为 Deployment；StatefulSet 仅在明确选择持久卷后开放。

## 7. 发布前校验

校验分为“阻断”和“警告”。阻断错误不能发起发布；警告可由有发布权限的用户确认继续。

| 校验 | 等级 | 说明 |
| --- | --- | --- |
| Namespace 可访问且用户有权限 | 阻断 | 防止越权发布 |
| 镜像格式、tag/digest 合法 | 阻断 | V1 不代替镜像仓库鉴权校验 |
| Secret/ConfigMap 键名合法且无重复 | 阻断 | Secret 明文不写入操作日志 |
| Service selector 与工作负载标签一致 | 阻断 | 防止无 Endpoint |
| 目标 Service 端口存在 | 阻断 | 防止路由指向无效端口 |
| 域名在同一入口中未冲突 | 阻断 | 按 host + path 检查 Ingress 冲突 |
| 公网模式存在可用 Ingress Controller | 阻断 | Traefik 不可用时禁止创建公网路由 |
| cert-manager / Issuer 就绪 | 阻断 | 启用 TLS 时必须满足 |
| DNS 解析到入口 | 警告 | V1 使用 DNS 查询检查；不阻止内网或预配置场景 |
| 未配置 livenessProbe | 警告 | V1 可允许但需明确确认 |
| 单副本生产发布 | 警告 | 不强制高可用，但提示风险 |

## 8. 运行状态与回滚

### 8.1 发布状态机

```text
draft -> validating -> applying -> waiting_ready -> verifying -> succeeded
                  \-> failed
succeeded -> rolling_back -> rolled_back
```

`waiting_ready` 使用 Deployment 状态、Pod Ready、Service Endpoint 和 Certificate Ready 判断。验证成功仅表示集群内资源状态满足预期；公网 DNS 传播和外部可达性在 V1 中显示为独立检查结果，不能伪造为成功。

### 8.2 回滚

回滚以“上一个成功 Release 的发布定义”为准：恢复镜像、replica、配置引用、Service 与路由声明。Secret 值不从旧版本恢复，除非该旧 Release 引用的 Secret 仍存在；若已删除，回滚必须阻断并明确指出缺少的 Secret。

Deployment 的 Kubernetes revision 仅作为诊断信息，不单独作为平台回滚依据，因为它不包含 Route、Certificate 和配置引用的完整快照。

## 9. 页面规划

“应用”一级模块下设置以下二级导航，资源级排障仍使用“基础设施”模块：

- `应用`：应用列表、创建应用、发布版本、单应用发布历史和回滚。
- `项目与环境`：Project 列表展示每个 Project 下的 Environment、Namespace 与应用数量，支持 Project 的创建、说明编辑和无关联记录时删除；点击项目后进入 `/applications/projects/:id` 管理该项目的 Environment。Environment 支持创建、编辑和无关联应用时删除，详情页左上提供返回项目列表的入口。后续在此扩展成员授权。
- `发布记录`：跨应用汇总 Release，作为查看异步发布进度和失败详情的统一入口。

### 9.1 应用列表 `/applications`

- 按 Project、环境、Namespace、状态筛选。
- 展示应用名、项目归属、当前版本、镜像、就绪副本、访问地址、最近发布时间、健康状态。
- 提供“创建应用”“发布新版本”“查看详情”和“回滚”操作。

项目可从已授权且启用的镜像仓库中设置一个默认仓库。应用发布时自动带出该值，用户仍可选择同项目已授权的其他仓库，或使用自定义完整镜像地址。

### 9.2 创建/发布向导 `/applications/:id/release`

采用固定步骤，不在一个表单中堆叠全部资源字段：

1. 基本信息：Project、环境、Namespace、应用名。
2. 工作负载：镜像、端口、副本、资源、探针、更新策略。
3. 配置：ConfigMap 和 Secret 的新增/引用。
4. 服务入口：Service 端口、访问模式、域名、路径、TLS 与 Issuer。
5. 预检与确认：展示渲染 YAML、变更摘要、阻断项、警告项。
6. 发布进度：按步骤展示实时状态、事件和失败详情。

### 9.3 应用详情 `/applications/:id`

- 从应用列表点击一条应用记录进入；左上提供“返回应用列表”。发布历史不在列表页内展开，避免改变列表布局。
- 概览：当前 Release、访问 URL、健康状态、资源用量、证书到期时间。
- 版本：发布历史、差异摘要、操作人、回滚入口。
- 运行：Deployment、Pod、事件、日志跳转、Service Endpoint。
- 配置：仅展示 ConfigMap；Secret 显示键名和引用关系，明文查看沿用现有二次确认与审计规则。
- 网络：域名、路由、TLS、DNS/证书检查结果。

### 9.4 发布详情 `/applications/:applicationID/releases/:releaseID`

- 从应用详情或跨应用发布记录点击一个 Release 进入；左上返回所属应用详情。
- 展示发布步骤、状态、执行详情和开始时间；非终态发布以固定间隔刷新状态。
- 发布失败时提供重试入口；已创建的发布提供回滚入口。操作完成后进入新建 Release 的详情页。

现有 `/workloads`、`/services`、`/configs`、`/routes`、`/certs` 继续保留，并新增“所属应用”筛选与跳转链接。

## 10. 后端边界与 API

新增 `internal/application` 领域服务，负责发布编排、预检、资源渲染和状态聚合；HTTP handler 只处理鉴权、请求校验和任务查询。Kubernetes 操作仍复用并扩展 `internal/k8s`。

建议 API：

```text
GET    /api/projects
POST   /api/projects
PUT    /api/projects/:id
DELETE /api/projects/:id
GET    /api/projects/:id/environments
POST   /api/projects/:id/environments
PUT    /api/projects/:id/environments/:environmentId
DELETE /api/projects/:id/environments/:environmentId
GET    /api/applications
POST   /api/applications
GET    /api/applications/:id
POST   /api/applications/:id/releases/validate
POST   /api/applications/:id/releases
GET    /api/applications/:id/releases
GET    /api/releases/:id
POST   /api/releases/:id/retry
POST   /api/releases/:id/rollback
```

发布操作应异步执行，接口立即返回 `release_id`。前端通过轮询 `GET /api/releases/:id` 获取步骤状态；后续可升级为 WebSocket/SSE。每一步记录到现有审计/操作日志体系。

## 11. 安全、权限与审计

- Project/Environment 是授权边界；用户只能发布到被授权的 Namespace。
- Secret 输入仅允许写入，响应与日志均不得包含明文。DNS API 凭据必须作为 Secret 管理。
- 发布、重试、回滚、删除应用、证书申请和路由变更均写入审计日志，记录操作者、目标、版本、结果和失败原因。
- 平台创建的资源只能更新带 `app.kubernetes.io/managed-by=cylism-manager` 标签的对象；发现同名但非平台托管资源时必须阻断并要求用户显式接管。
- 发布前对镜像仓库地址实施允许列表或私有仓库凭据策略，避免任意镜像来源。

## 12. 分期实现

### Phase 1：可发布的无状态 HTTP 服务

- Project、Environment、Application、Release 数据模型。
- Deployment + ClusterIP Service + ConfigMap/Secret 引用。
- 单域名 Ingress/IngressRoute，复用已有 cert-manager Certificate 能力。
- 预检、异步步骤记录、发布历史和回滚。
- 应用列表、应用详情、发布向导。

### Phase 2：运维闭环

- Pod 日志、Kubernetes Event、资源利用率、发布后连通性检查。
- DNS 解析检查、证书续期/到期告警、失败重试。
- Namespace 配额、LimitRange、NetworkPolicy 模板。

### Phase 3：交付集成与高级发布

- 镜像仓库凭据和镜像 digest 固化。
- Git/CI 触发、发布审批、环境晋级。
- HPA、PVC/StatefulSet、蓝绿/金丝雀策略。

## 13. 验收场景

1. 用户可在一个向导内创建 `order-api`，平台依次创建配置、Deployment、Service、Certificate 和路由，并显示 `https://api.example.com`。
2. Service selector 不匹配时，预检阻断发布并指出标签差异。
3. `kube-system/traefik` 不可用时，公网入口发布被阻断；仅集群内 Service 发布仍可继续。
4. Certificate 未就绪时，发布停留在 `waiting_ready`，展示 cert-manager 事件，不创建指向无 TLS Secret 的 HTTPS 路由。
5. 已成功的 Release 可回滚到上一成功版本，且发布记录和审计完整保留。
6. 用户访问已有资源页时，可从带 Cylism 标签的资源跳转回所属应用；未托管资源仍可正常查看但不能被发布中心静默覆盖。

## 14. 待确认事项

- 公网入口的部署形态：单台带公网 IP 的 Traefik 节点、云负载均衡器，还是 Tailscale Funnel。
- 域名 DNS 托管商范围：V1 仅手工校验，还是优先支持 Cloudflare DNS-01。
- 生产环境是否要求发布审批，以及审批由平台内置还是对接现有系统。
- 已实现外部镜像仓库的项目授权、加密凭据存储与 `imagePullSecret` 下发；后续补充连接验证、digest 解析和私有 CA 的节点级配置。
- 多集群是否在近期范围内；若是，Environment 需绑定明确的 `cluster_id` 并扩展权限模型。
