## Context

平台当前只支持一个受管 OCI Registry，并持久化其 endpoint、加密 Basic Auth 凭据和关联的 Image Registry / Node Registry Mirror。页面能够管理部署资源，但未读取 Registry Distribution API，因而无法展示或维护已推送制品。项目已使用 `go-containerregistry` 验证镜像仓库连通性；凭据的解密边界位于 Registry service，适合由后端代替浏览器访问私有 Registry。

Docker Distribution 的删除单位是 manifest digest，不是独立 Tag。删除一个 Tag 对应的 digest 可能同时移除同 digest 的其他 Tag；删除仓库需要逐个删除其 manifest digest。因此任何删除都必须先汇总 Registry 影响和平台内引用，避免破坏仍可部署、重试或回滚的镜像。

## Goals / Non-Goals

**Goals:**

- 让用户在一个页面中切换受管 Registry 的概览和镜像目录。
- 提供仓库、Tag、digest、媒体类型、平台信息与完整拉取地址的分页浏览及搜索。
- 让服务端以受管凭据访问 Registry，绝不将明文凭据、授权头或内部 Secret 返回给前端。
- 支持删除未被平台 Release 或上线模板引用的 Tag、manifest 与仓库，并在删除前显示 digest 共享 Tag 和引用阻断原因。
- 对目录读取和删除执行建立可测试的超时、分页、错误映射和审计边界。

**Non-Goals:**

- 不提供镜像 push、跨 Registry 复制、漏洞扫描、垃圾回收、保留策略或镜像构建。
- 不管理 Kubernetes 节点运行时缓存中的镜像；镜像页只反映受管 OCI Registry 的存储内容。
- 不在前端保存、展示或导出 Registry 密码。
- 不允许绕过平台引用检查的强制删除。

## Decisions

### 1. 将配置与镜像管理组织为同一页面的两个页签

使用现有 `SectionTabsHeader` 呈现“自托管制品库 / 概览 / 镜像”。概览保留当前状态、PVC 与资源配置；镜像页按需加载目录数据，并拥有独立的搜索、刷新、空态和删除反馈。这与操作历史等页面的标题节奏一致，同时避免将配置和制品清单堆叠为一个长页面。

备选方案是保持 `PageHeader` 并在下方添加卡片内页签。该方案不能与当前页面标题体系对齐，且容易让页签成为嵌套卡片，因此不采用。

### 2. 由服务端代理 OCI Distribution 目录和 manifest 查询

新增一个 Registry catalog service，通过受管 Registry ID 读取 endpoint 和加密凭据，在服务端创建带超时的 OCI 客户端。目录查询使用分页 catalog；仓库 Tag 查询仅在展开该仓库时执行；manifest 元数据按当前页 Tag 有界并发读取。响应仅包含仓库名、Tag、digest、媒体类型、平台和可展示的拉取引用。

备选方案是前端直接调用 `/v2/`。浏览器会处理跨域、TLS 与 Basic Auth，并暴露长期拉取凭据，不符合受管凭据边界，因此不采用。

### 3. 删除按 manifest digest 预检、确认并执行

删除 Tag 前，服务端解析其 manifest digest，列出同仓库中指向该 digest 的所有 Tag，并检查匹配该仓库和 digest/Tag 的 Release 与上线模板。前端的确认对话框明确列出将受影响的 Tag；存在平台引用时 API 返回冲突并拒绝操作。删除仓库时，服务端先枚举全部 digest 并聚合引用与共享 Tag；仅全部 manifest 均无引用且用户明确确认仓库名后才逐项删除。

备选方案是对 `/v2/<repository>/manifests/<tag>` 直接发 DELETE。Distribution 的实际删除行为由 digest 决定，直接按 Tag 删除会隐式破坏同 digest 的其他 Tag，且无法保护已有发布，因此不采用。

### 4. 引用检查覆盖可重新部署的持久化定义

引用检查使用持久化 Release 和应用上线模板中规范化后的镜像引用，而不只检查当前运行 Pod。这样可防止删除历史重试、回滚或模板再次发布仍需要的制品。检查按 Registry endpoint、仓库名和 digest/Tag 精确匹配；无法从镜像引用安全解析的记录保守地视为引用并返回可识别原因。

备选方案是只检查 Kubernetes 当前工作负载。它遗漏未运行但仍可发布的模板和历史 Release，因此不采用。

### 5. 删除行为使用现有审计体系

成功与失败的删除操作记录受管 Registry、仓库、digest、请求 Tag、受影响 Tag 数和引用检查结果；审计元数据不得包含密码、Authorization 头或 manifest 内容。删除后使镜像页当前数据失效并重新读取目录。

备选方案是仅记录浏览器事件。该方案无法为服务端实际删除提供不可抵赖记录，因此不采用。

## Risks / Trade-offs

- [大型 Registry 的 catalog / Tag 请求耗时长] → 使用上游分页、服务端超时、有限并发和按页懒加载；不在概览页自动扫描。
- [同 digest 的多 Tag 被一并删除] → 预检返回完整受影响 Tag 清单，确认文案明确该行为，后端在执行前再次校验 digest。
- [镜像字符串与存储引用不一致] → 使用规范化镜像引用解析；不能可靠判断时保守阻止删除并显示记录标识。
- [Registry 不可用或认证失效] → 将上游错误映射为可操作的状态，不把失败显示为空目录；删除端点在不可用时不执行。
- [并发推送或删除造成目录过期] → 预检响应携带 digest；删除请求必须提交该 digest，并在远端返回不存在时提示目录已变化后刷新。
- [高风险删除缺乏追踪] → 使用现有审计 middleware / service 写入成功、拒绝和失败结果。

## Migration Plan

1. 发布只读 catalog API 与镜像页；现有 Registry 配置、关联记录和镜像拉取流程不变。
2. 发布删除预检与执行 API，并以服务端测试覆盖共享 digest、引用阻断、远端失败和审计。
3. 在前端接入确认对话框；删除后刷新当前仓库和目录摘要。
4. 回滚时移除新页面入口和 API 路由即可；不涉及数据迁移或 Registry 数据格式变更。

## Open Questions

- None.
