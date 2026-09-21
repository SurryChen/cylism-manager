## Context

`web/src/styles/theme.css` 当前通过通配选择器为所有元素配置固定可见的细滚动条，`body` 也始终保留纵向滚动条。移动导航已有局部 `is-scrolling` 样式，但没有覆盖文档、桌面侧栏、弹窗、表格和终端等其他容器。

## Goals / Non-Goals

**Goals:**

- 使任意可滚动容器在发生实际滚动时显示滚动条，停止滚动后短暂延迟再隐藏。
- 覆盖页面根滚动和嵌套 `overflow: auto/scroll` 容器，包括后续动态挂载的内容。
- 保持键盘、鼠标滚轮和触控板滚动可用，且不引入布局宽度抖动。
- 在组件卸载时清理监听器与所有待执行计时器。

**Non-Goals:**

- 不模拟或替代浏览器/操作系统的原生 overlay scrollbar。
- 不改变滚动条尺寸、页面滚动逻辑、终端滚动缓冲区或滚动锁定行为。
- 不逐页手工添加事件处理器，也不提供用户偏好开关。

## Decisions

### 使用根应用安装的捕获式 scroll 监听

根应用通过 composable 向 `document` 注册 `capture: true` 的被动 `scroll` 监听。`scroll` 本身不冒泡，但捕获阶段可以观察到嵌套滚动容器，因此一个监听器即可覆盖弹窗、抽屉、表格和后续动态挂载的区域。处理器给事件目标元素添加 `is-scrolling`，并在最后一次事件后 700ms 移除；文档滚动使用 `document.documentElement` 作为样式目标。

备选方案是只在 CSS 中隐藏滑块。这无法在滚动期间可靠地恢复可见性；逐页绑定事件则会遗漏独立弹窗与未来页面，维护成本也更高。

### 通过 CSS 透明滑块维持布局，而不是移除滚动条

默认状态保持既有窄滚动条宽度与 `scrollbar-gutter: stable`，仅将滑块和轨道设为透明。`is-scrolling` 恢复当前主题的 `--text-muted` 滑块色；Firefox 使用 `scrollbar-color`，WebKit 系浏览器使用伪元素规则。

备选方案是 `scrollbar-width: none`。它会让 Firefox 完全移除滚动条，并可能与稳定 gutter 和平台可发现性产生不一致，故不采用。

### 将可见性逻辑做成独立 composable

`useTransientScrollbarVisibility` 封装监听器、元素到计时器的映射以及卸载清理。根应用只负责安装；测试可以直接验证行为与资源回收。

备选方案是在 `App.vue` 内内联事件处理逻辑。该方式不利于复用与隔离测试，也会使根组件承担不相关的交互细节。

## Risks / Trade-offs

- [浏览器对 scrollbar 伪元素的支持不同] -> 同时提供 Firefox 标准属性和 WebKit 伪元素；不支持的浏览器保留其原生表现。
- [高频 scroll 事件创建大量计时器] -> 每个活跃元素最多保留一个计时器，连续事件仅重置计时器。
- [隐藏滑块降低静止状态下的可发现性] -> 保留滚动空间和所有输入方式；实际滚动立即提供可见反馈。
- [终端或第三方组件拥有专有 viewport] -> 仅依据发生的原生 scroll 事件添加类名，不修改其 overflow、DOM 结构或滚动缓冲区。

## Migration Plan

1. 先添加 composable 的失败测试，固定计时器时序与清理约束。
2. 实现 composable、根应用安装和全局主题样式。
3. 运行前端单测与生产构建，在页面、弹窗和窄屏抽屉中手动验证。
4. 回滚仅需移除根应用安装和样式规则；不涉及数据迁移或 API 兼容性。

## Open Questions

- 无。700ms 作为当前固定隐藏延迟；后续若有可用性反馈，可独立调整为设计 token。
