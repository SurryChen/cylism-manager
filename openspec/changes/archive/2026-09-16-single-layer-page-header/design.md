## Information Architecture

记录与系统沿用现有两层导航，不再复制一个页面内领域层：

```text
顶部一级导航：记录与系统
└── 左侧二级导航
    ├── 审计日志       → /audit
    ├── 操作历史       → /operations
    └── 系统设置       → /settings/system
        └── 页面内分类：安全与访问 | 平台入口 | 发布与更新
```

侧栏切换的是路由；系统设置 tabs 切换的是同一路由内的设置分类。两者职责不同，因此系统设置可以保留一组同行 tabs，但审计日志和操作历史不能再被包进一个共同的页面级 tabs 容器。

## Header Contract

每个路由内容只能选择下列一种主标题结构：

| Route type | Component | Composition |
| --- | --- | --- |
| 无页面内分类 | `PageHeader` | 一个 `h1`，可选说明、右侧统计或操作 |
| 有页面内分类 | `SectionTabsHeader` | 一个 `h1` 与一组同级 tabs，固定下边框基线 |

`AuditLogs` 与 `OperationHistory` 使用前者。`SystemSettings` 使用后者；当前设置分类的内容视图只渲染分类标题、说明和功能内容，不能再包含第二个 `PageHeader` 或 `SectionTabsHeader`。

页面之间的一致性通过共享 token 和组件 CSS 实现：相同的内容起点、标题字号范围、操作区对齐和移动端换行规则。这里的“一致”不要求常规标题和 tabs 标题具有机械相同的像素高度，因为说明文本与同行 tabs 承载的信息不同；它要求每个页面只有一个明确、稳定的标题层。

## Boundaries

- 不创建 `RecordsHub`，不使用 `KeepAlive` 在父级中交换这三个页面，也不把 `tab` 查询参数作为三个路由的替代品。
- 不改变 `App.vue` 中“记录与系统”侧栏的三个入口或它们的独立活跃态。
- `SectionTabsHeader` 不用作内容区的二次导航容器；仅由拥有页面内分类状态的路由使用。
- 对现有组件做抽取或 token 收口时，不应迁移 API 请求、筛选状态、轮询或业务行为。

## Verification Strategy

- 组件测试验证两个 header 都只有一个语义 `h1`，并且 tabs 与标题处于同一标题区。
- 页面测试验证审计日志、操作历史和系统设置各自只出现一套路由级标题结构。
- 路由与导航测试验证三条现有路径保持直接可访问，并由左侧导航分别激活。
- 跑完整前端测试、前端生产构建、`go test ./...`、`go build ./...`、OpenSpec 严格校验和 diff 检查。
