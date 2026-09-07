## Context

当前 `ReleaseSpec` 的兼容字段将 Service 建模为单个隐式 TCP 端口，渲染器生成一个 `ClusterIP` Service；`EndpointSpec.Exposure=public` 总会创建 HTTP Ingress。模板的 ConfigMap 和 Secret 使用 `envFrom`，PVC 是唯一可挂载的卷类型。

Kubernetes Service 原生支持 UDP，但 HTTP Ingress 不能表示任意 UDP 或 QUIC 四层转发。应用应明确选择 HTTP 路由或 Service 直出，而非根据镜像名或端口猜测协议。

## Decisions

### 1. 以显式、带默认值的 Service 字段扩展发布定义

`ServiceSpec` 保留原有 `port`、`target_port`、`protocol` 和 `node_port` 作为历史单端口兼容字段，并新增 `ports` 列表。列表中每项有名称、Service 端口、目标容器端口、TCP/UDP 协议及可选 NodePort。Service 级字段为：

- `type`: `ClusterIP`、`NodePort` 或 `LoadBalancer`，默认 `ClusterIP`。
- `external_traffic_policy`: 仅 `NodePort`/`LoadBalancer` 时允许 `Cluster` 或 `Local`。

规范化时，空 `ports` 列表由历史字段生成一个 TCP 端口；非空列表完全决定渲染结果。端口名称在一个 Service 内唯一，且每个目标端口与协议组合会声明为 ContainerPort。历史 JSON 不含新字段时使用默认值，保证历史模板和发布快照继续工作。

Service 类型和外部流量策略是 Kubernetes Service 的全局属性，不能针对单个端口分别配置；NodePort 则属于各个 ServicePort。HTTP Ingress 仍只路由 TCP，默认选择端口列表中的首个 TCP 端口。没有 TCP 端口的模板不得绑定 HTTP Ingress。

### 2. 将 HTTP Ingress 与 Service 直出区分为入口模式

Endpoint 增加 `routing_mode`：

- `http_ingress`: 现有域名、路径和 TLS 入口，要求 TCP Service。
- `service`: 不创建 Ingress。公网性由 NodePort 或 LoadBalancer Service 提供，不要求 HTTP 路径。
- `cluster`: 仅 ClusterIP 访问。

旧 Endpoint 没有 `routing_mode` 时视为 `http_ingress`。UDP Service 不得绑定 `http_ingress`，以阻止生成无效 Ingress。Service 直出可以关联域名证书，但该关联只服务于应用自身 TLS，不会生成 HTTP 路由。

### 3. 新增只读对象文件投影

新增模板 `FileMountSpec`，每项包含来源类型（ConfigMap 或 Secret）、同命名空间来源名称、可选 key 列表、容器挂载路径和只读标记。

渲染器为来源对象生成 Kubernetes volume 和 volumeMount。Secret 与 ConfigMap 的默认模式为只读，不提供主机路径、subPath 写入或跨命名空间引用。来源对象由平台在发布预检阶段检查存在性、类型和请求 key；模板自身的 `config`、`secrets` 仍保留为环境变量来源。

### 4. UI 以通用资源语义表达能力

模板编辑器增加“网络与 Service”区域，字段为传输协议、Service 类型、端口、NodePort 和流量策略。HTTP Ingress、Service 直出和仅集群内访问作为入口模式选择，而不是以某个应用的名称区分。

编辑器增加“配置文件挂载”区域，用来源类型、名称、key、挂载路径和只读状态表示投影。现有 ConfigMap/Secret 环境变量区域不改变。

HTTP Ingress 仍通过现有域名绑定界面管理；选择 Service 直出的应用不显示 HTTP URL，而展示已分配的外部地址和端口。

模板编辑器使用单列分区式宽屏工作区：基础信息、服务部署、服务网络、配置存储和资源健康检查分别呈现；保存操作固定在编辑器底部且位于滚动表单外。条件字段继续按 Service 类型、协议和存储配置渐进显示，避免将不适用字段混入主要流程。

启动命令、启动参数以及 ConfigMap/Secret 环境变量使用可增删的逐行输入，而不是换行分隔的大文本框；提交时仍转换为现有的数组或键值映射，保持 API 兼容。

### 5. 校验和运行时边界

- UDP 不允许 HTTP Ingress。
- ClusterIP 不允许指定 NodePort 或 externalTrafficPolicy。
- NodePort 的端口必须位于集群配置的 Service NodePort 范围，或为零以交由集群分配。
- 所有投影来源必须位于应用 Namespace，名称和挂载路径必须经过 Kubernetes/DNS 与绝对路径校验。
- HTTP 和 TCP probe 不能被解释为 UDP 健康检查；UDP 应用只有在显式配置合适的 probe 时才渲染，默认依靠 Pod Ready 状态和 Service Endpoint。

## Alternatives Considered

- **让应用读取 Kubernetes API 中的 Secret**：引入 SDK、ServiceAccount RBAC 和集群耦合，且不适合一般业务容器。
- **将 Secret 一律注入环境变量**：无法给需要证书文件的应用使用，且无法随 Secret 投影更新。
- **将 UDP 路由模拟为 HTTP Ingress**：HTTP Ingress 不透传任意 UDP/QUIC 会话，协议语义错误。
- **为单一应用新增专用模板字段**：会让平台资源模型与应用实现绑定，无法复用于 DNS、实时通信、游戏和其他四层服务。

## Risks and Mitigations

- 集群未配置 LoadBalancer：预检显示可操作错误，并允许用户改用 NodePort。
- Service 直出的外部地址由供应商异步分配：发布完成后单独展示 Service status 的地址，不能伪造 HTTP URL。
- Secret 文件投影有权限风险：只允许受管命名空间内的命名来源，并在 API 层执行应用归属与授权校验。
- 旧端点语义不同：缺失入口模式一律回退既有 HTTP Ingress 语义。
