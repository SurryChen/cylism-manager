## Context

当前文件挂载模型只表达 `source_type`、`source_name`、`key` 和容器路径。它在 Kubernetes 渲染和发布预检中已经具备安全边界，但用户必须自行准备来源资源。配置页面只能读取资源；证书页面和受管域名页面可以创建 cert-manager Certificate 和 TLS Secret，却没有模板内的选择和状态反馈。

## Decisions

### 1. 模板保存引用，不保存资源内容

模板继续保存文件投影引用，不复制 ConfigMap 或 Secret 值。资源由 Kubernetes 在应用 Namespace 内保存，Secret 永不返回到模板、发布快照、审计详情或普通列表接口。

创建资源的入口可以出现在模板编辑器内，但创建的仍是独立的 Namespace 资源。用户可在资源管理页复用、编辑和查看引用，模板不会在每次发布时重复创建资源。

### 2. 资源选择器与快捷创建复用同一资源表单

模板的“文件资源”行使用来源类型、资源选择器、key 选择器和挂载路径。资源选择器只列出当前应用 Namespace 中的匹配来源；选择资源后，key 列表从资源元数据加载。

每种来源提供“新建 ConfigMap”或“新建 Secret”快捷操作，打开与配置页相同的资源编辑表单。成功创建后关闭弹窗、刷新选择项并自动选中。资源编辑和删除的完整入口仍位于配置页，模板内只承担当前任务所需的快捷创建。

首期可创建和编辑 ConfigMap 及 `Opaque` Secret。系统管理 Secret、ServiceAccount Token、dockerconfigjson 和 cert-manager 生成的 `kubernetes.io/tls` Secret 不在通用编辑器中开放。

### 3. TLS 以受管域名/证书为来源

模板提供通用“TLS 证书”辅助项，而不是要求用户填写 `tls.crt` 与 `tls.key` 的资源和 key。它列出当前环境中状态为 Ready 的受管域名及其 TLS Secret；选择后生成两个普通 Secret 文件投影，分别指向 `tls.crt` 和 `tls.key`。

“申请证书”在模板弹窗内复用受管域名申请的域名、环境和 ClusterIssuer 语义。证书签发仍通过 DNS-01/cert-manager 完成，创建的域名和 Certificate 保持原有生命周期。模板可以保存引用尚未就绪的证书，但发布会被阻断，直到 Certificate Ready 且目标 TLS Secret 包含两个必需 key。

该辅助项适用于任何需要标准 Kubernetes TLS Secret 的容器，不绑定 Hysteria、HTTP Ingress 或特定协议。它不会创建 HTTP Ingress。

### 4. 依赖状态和删除保护

应用详情新增“运行依赖”区域，展示模板使用的 ConfigMap、Secret 和 TLS Certificate：名称、来源、键、就绪/缺失状态及跳转操作。模板保存、发布预检和应用详情使用同一份依赖解析逻辑。

删除用户管理的 ConfigMap 或 Opaque Secret 前，平台检查模板引用和有效发布快照引用；存在引用时拒绝删除并返回引用应用和模板。删除受管域名或 Certificate 前，也检查其 TLS Secret 是否被模板引用。已由 cert-manager 管理的 TLS Secret 不允许在通用 Secret 页面直接删除。

### 5. 权限与审计

所有读写、选择与删除均受应用项目/环境权限和 Namespace 边界约束。写操作记录审计日志，但绝不记录 Secret 值。Secret 编辑采用“未填写则保持原值”，后端响应只返回键名与元数据。

## API Shape

配置 API 按 Namespace 和资源类型提供列表、详情、创建、更新、删除；列表和模板选择器均不返回 Secret 值。模板依赖 API 返回来源元数据、请求的 key、可用性和 TLS Certificate 状态。

模板内发起证书申请调用既有受管域名创建流程；成功响应包含域名、Certificate 名称、TLS Secret 名称和当前状态，前端据此建立挂载引用。

## Alternatives Considered

- 让模板直接保存完整 YAML 或 Secret 值：模板复用与资源生命周期会混淆，也会扩大明文泄漏面。
- 强制所有资源先在资源页面创建：安全边界清晰，但用户需频繁离开上线流程，且当前资源页甚至没有写能力。
- 在模板中直接创建 TLS Secret：跳过 cert-manager，无法自动签发、续期和追踪，且容易误把私钥交给普通表单。
- 把 TLS 做成应用专用字段：无法复用于 TLS 终端、MQTT、gRPC 等通用工作负载。

## Risks and Mitigations

- 证书签发异步：允许模板保存，发布前显示明确的等待原因和签发过程跳转。
- Secret 修改影响运行中工作负载：详情显示引用关系；更新提示用户应用是否支持热加载，平台不假设所有应用会重载配置。
- 资源删除影响回滚：删除保护检查模板和发布快照引用，不静默删除仍被依赖的资源。
- 快捷创建权限扩大：与资源页使用同一后端鉴权、Namespace 校验和审计路径。
