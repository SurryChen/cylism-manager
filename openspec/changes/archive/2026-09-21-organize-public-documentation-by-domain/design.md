## Context

VitePress 迁移已建立 `docs/.vitepress/`，但现有公开内容仍以“快速开始、安装部署、使用指南、运维与排障、安全、开发与贡献”组织。产品实际已有受管 Registry、Registry Proxy、节点镜像源、Chart 仓库、集群资源、日志、告警、审计和 Agent 等能力。用户已确定产品文档采用七个业务域；该结构是文档的阅读模型，不要求修改应用导航。

## Goals / Non-Goals

**Goals:**

- 让操作员按要管理的产品能力定位文档。
- 每页从真实界面和现有行为出发，说明入口、前置条件、关键操作、风险和关联页面。
- 让产品截图可逐页补充而不阻塞首版文字文档。
- 保留已有部署与安全资料的稳定 URL。

**Non-Goals:**

- 不把每个页面内 Tab 都拆成侧栏一级入口。
- 不以文档目录重命名前端路由或提供不存在的管理页面。
- 不在本次建立版本化 API 参考、教程视频或多语言内容。

## Decision

### 1. 产品域目录和页面粒度

目录按以下规范路径建立。括号内容是同页小节，不单独产生侧栏叶子：

```text
docs/
  index.md                                      概览：平台健康、待处理告警、近期发布、关键操作

  application-delivery/
    projects-and-environments.md
    application-workspace.md
    releases.md
    domains-and-access.md

  supply-chain/
    managed-registry.md
    registry-proxy.md
    node-registry-mirrors.md
    helm-chart-repositories.md

  platform/
    servers-and-terminal.md
    cluster-and-system-components.md
    kubernetes-resources.md                     工作负载、服务、配置、Pod 终端
    network-and-certificates.md                 路由、站点、证书
    storage.md                                  PVC、导入、备份、迁移

  operations/
    metrics.md
    logs.md
    alerts.md
    disk-growth.md
    maintenance-and-troubleshooting.md

  governance/
    audit-logs.md
    operation-history.md
    system-settings.md
    database-management.md
    identity-permissions-security.md

  automation/
    agent-assistant.md
    automated-operations.md
```

“应用镜像仓库”不另设产品域叶子页，而在 `application-delivery/releases.md` 的镜像来源章节中说明，借此与 `managed-registry.md` 的平台受管 OCI Registry 区分。部署、配置和安全等既有页面保留在原路径：`getting-started.md`、`installation/**`、`operations/configuration.md`、`security.md`、`development/contributing.md`。

### 2. 导航模型

顶部导航显示七个产品域；产品页面侧栏按当前域显示对应页。首页和每个产品域页面提供“开始使用与部署”辅助入口，指向既有安装、配置、安全和贡献资料，不在产品域列表中混排。

`operations/` 继续承载现有配置、日常运维和排障地址；其产品域页面与辅助资料在同一 VitePress 前缀下通过明确分组区分，避免链接迁移。内容标题使用用户确定的中文术语，均以现有 UI 和公开 API 行为为准。

### 3. 每页的固定内容骨架

所有新增产品页采用一致顺序：用途与边界、进入位置、准备条件、核心操作、状态与风险、关联页面。复合页使用 H2/H3 覆盖其列出的子能力，例如 Kubernetes 资源页包含工作负载、服务、配置和 Pod 终端；不把这些子能力拆为独立文件。

没有独立 UI 的“身份、权限与安全”仅阐明当前管理员登录、Kubernetes/SSH 凭据、Secret、Agent 授权和审计边界；“自动化操作与执行记录”仅阐明 Agent 当前操作记录与审批语义，不虚构全局任务中心。

### 4. 截图占位组件

在 VitePress 主题中注册 `ScreenshotPlaceholder`。需要界面辅助的新增页引用：

```md
<ScreenshotPlaceholder
  title="节点镜像源"
  description="规则列表、验证状态和节点应用入口"
  filename="supply-chain/node-registry-mirrors.png"
/>
```

组件显示简洁的虚线图框、标题、说明和建议相对路径，并具备可访问标签。后续替换时，维护者将组件替换为同一路径下的 Markdown 图片；组件不得伪装成真实界面或使用虚构数据。

### 5. 迁移与质量检查

先新增页面和导航，再更新首页链接。保留原页面及 URL，不做破坏性移动。检查脚本验证所有新页面存在、每个产品域具有导航入口、`ScreenshotPlaceholder` 引用的文件名仅位于 `assets/screenshots/` 下，且站点能成功构建。

## Alternatives Considered

### 按前端一级导航逐字复制文档

前端导航会随产品操作频率调整，且不适合容纳部署、配置和安全资料。文档以用户给出的七域为主、辅助资料独立保留，避免两者被迫完全同步。

### 为每个子功能建立单独页面

将工作负载、服务、配置、Pod 终端等全部拆开会造成超过二十个低信息页和过长侧栏，不采用。

### 使用 Markdown 注释作为截图占位

注释不利于读者和维护者发现缺失截图，也无法统一检查。采用可见组件，并在后续以真实 Markdown 图片替换。

## Risks / Trade-offs

- [首版页面较多] -> 使用一致骨架和聚合页，先覆盖实际操作入口与风险，避免重复 UI 描述。
- [文档与产品后续不一致] -> 每页写明真实入口和关联页面，并用站点检查守住目录与链接。
- [可见占位影响正式感] -> 占位样式克制，仅在确实需要界面辅助的页面出现；真实截图补齐后可直接替换。
- [既有路径被忽略] -> 顶部和首页保留部署与配置辅助入口，旧链接不移动。

## Migration Plan

1. 为新目录、导航和截图组件增加失败的站点检查。
2. 注册截图占位组件，创建七域页面并根据现有功能编写正文。
3. 更新首页、顶栏和按域侧栏，同时保留既有支持页面和 URL。
4. 执行文档检查、VitePress 构建、链接检查、桌面和移动浏览器检查与 `git diff --check`。

## Open Questions

- None.
