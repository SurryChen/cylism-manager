## Component Contract

`SurfaceCard` 是一个仅承担视觉表面和结构区域的 Composition API 组件：

| Contract | Purpose |
| --- | --- |
| `as` | 选择语义容器，默认 `section`；卡片在重复条目中可用 `article`。 |
| `padding` | `md`（默认）、`sm`、`none`，只管理面板外层内边距。 |
| `interactive` | 为可点击卡片提供明确的 hover/focus-visible 外观，不添加 click 行为。 |
| `header` slot | 放置标题、说明等卡片头部内容。 |
| `actions` slot | 放置卡片级按钮，并与标题区稳定对齐。 |
| default slot | 由调用页面完全拥有具体业务内容。 |

组件不接受 title、loading、error、data 或领域变体 props。标题文案、按钮可用性、请求状态和卡片内部网格仍由使用方负责。

## Styling

基础表面沿用已有 Glass UI token：`--surface-glass`、`--border`、`--shadow-soft` 和 `--radius-panel`。模糊效果使用已有降级策略；组件的变体只控制结构性内边距、头部布局和交互焦点，不覆盖内容区的分隔线。

Registry Proxy 的实例属性网格继续由视图局部样式负责，其细分隔线不能因更换外壳而丢失。自托管 Registry 的目录表格继续使用紧凑的数据表，而非转为嵌套卡片。

## Adoption

先迁移 `ManagedOCIRegistries` 与 `RegistryProxyWorkspace` 中的结构性透明面板。迁移只替换容器标签和共享外壳 class；不得改变 API 调用、Tab 路由、默认仓库选择、Proxy 状态判定或操作按钮行为。

保留全局 `.card`、`.metric` 的现有契约。本次不批量改造其他视图，避免扩大纯视觉组件变更的回归面。

## Directory Boundary

`web/src/components/` 只放置跨页面、无领域 API 所有权的组件。`RegistryProxyWorkspace` 持有 Registry Proxy 的 REST 请求、集群节点读取、表单状态、诊断与缓存清理操作，因此它是交付领域工作区，而不是共享组件。

它与同域测试一起迁移到 `web/src/views/delivery/RegistryProxyWorkspace.vue`；`ManagedOCIRegistries` 通过 `./delivery/RegistryProxyWorkspace.vue` 使用它。此次仅移动文件与修复相对导入，不改变路由、请求、数据状态或外部接口。

目录盘点结论：目前 `components/` 的 `BaseModal`、`ConfirmDialog`、`EmptyState`、`FeedbackBanner`、`PageHeader`、`SectionHeading`、`SectionTabsHeader`、`SelectMenu` 与 `SurfaceCard` 均为跨领域 UI 原语。监控、运行时、集群与资源领域的页面级子组件已经位于各自 `views/<domain>/` 下，无其他需要在本次迁移的已知错位文件。

## Records and System Adoption

记录与系统的三个入口使用 `SurfaceCard` 作为主结构面板：

| Page | Adopted surface | Preserved content |
| --- | --- | --- |
| Audit Logs | 筛选、审计表格与分页的共同容器 | 查询参数、筛选弹窗、行详情与分页行为 |
| Operation History | 筛选、操作表格与分页的共同容器 | 查询参数、状态标签与分页行为 |
| System Settings | 安全与访问、平台入口、发布与更新的各自主设置面板 | Tab 路由、KeepAlive、表单及异步操作状态 |

这些面板默认使用 `SurfaceCard` 的常规内边距。现有 `.audit-card`、`.operation-card` 及设置页局部 class 继续只承担内容布局，不能复制玻璃背景、边框、阴影或圆角定义。表格仍位于单一结构面板中，禁止给每个记录行追加卡片外壳。

## Alternatives Considered

1. **只增加 `.card--*` 修饰类**：改动最小，但标题/操作区结构仍分散在各视图，难以测试共享结构与可访问焦点态。
2. **创建 `RegistryCard`**：会把 Registry 专属信息模型编码到通用层，无法服务其他领域，不采用。
3. **一次迁移所有页面**：覆盖面过大且难以验证，保留为未来按页面渐进迁移。
4. **将 Proxy 工作区留在 `components/`**：会误导维护者将其视作可跨领域复用的原语，且违反既有组件边界规范，不采用。
5. **仅保留 `.card` class**：视觉 token 虽相同，但页面不会采用可测试的共享结构入口，后续卡片头部和间距变体仍会继续分散，不采用。

## Risks and Mitigations

- **新增包装组件改变局部布局**：`padding=none` 明确保留 Proxy 网格的零外边距需求，并通过视图测试保护内部结构。
- **样式与主题脱节**：组件只引用语义 token；组件测试和生产构建覆盖导入与渲染。
- **交互态语义不清**：`interactive` 仅提供视觉反馈和焦点环，不擅自赋予卡片按钮语义或注册点击事件。
- **文件迁移遗漏导入**：通过迁移后的组件测试、制品库视图测试与 Vite 构建验证所有相对路径。
- **记录或设置工作流回归**：迁移仅替换外层容器；通过各页面既有筛选、分页、Tab 与表单测试保护行为。

## Verification Strategy

- 先写组件失败测试，覆盖语义元素、结构插槽、变体 class、交互焦点标识与 token 驱动的卡面 class。
- 更新两个 Registry 工作区测试，断言采用共享组件且保留其业务内容结构。
- 更新 Proxy 工作区与制品库视图导入，验证领域组件不再位于共享目录。
- 更新记录与系统页面测试，断言主结构面板采用 `SurfaceCard` 并保留页面行为。
- 执行聚焦前端测试、完整前端测试、Vite build、`go test ./...`、`go build ./...`、OpenSpec 严格校验和 `git diff --check`。
