## 实现任务

### 1. Design Tokens 重定义
- [x] 更新 `App.vue` 中 `:root`/`.theme-dark`/`.theme-light` 全部 CSS 变量
- [x] 色调：暖灰底 + 紫蓝强调色（#7c6ff7）
- [x] 亮/暗两套完整变量（bg/surface/raised/hover/border/text/accent/semantic）

### 2. 组件统一样式
- [x] 卡片：8px 圆角 + 1px border + subtle 阴影 + 过渡
- [x] 按钮：6px 圆角 + 200ms 过渡 + focus-visible ring
- [x] 模态：12px 圆角 + 8px/32px 阴影 + 32px padding + backdrop-filter
- [x] 表格：行 hover 0.15s ease + tr transition
- [x] 徽章：4px 圆角 + 统一配色
- [x] 表单：6px 圆角 + 3px accent-glow focus ring

### 3. 滚动条 overlay
- [x] 全局 scrollbar-width: thin + scrollbar-color
- [x] webkit-scrollbar 6px + 透明 track + 圆角 thumb
- [x] hover 时 thumb 变亮

### 4. 亮色主题适配
- [x] Login.vue 硬编码 rgba 颜色替换
- [x] tab-active 在亮色模式 `color: #fff`（两侧相同，无需特殊处理）
- [x] overlay 背景色使用变量 `--overlay-bg`

### 5. 动效打磨
- [x] 全局 `transition: 0.2s ease` 应用于按钮/导航/卡片/表格
- [x] Modal overlay 添加 `backdrop-filter: blur(4px)`
- [x] 按钮 focus-visible 状态

### 6. 响应式完善
- [x] 768px 以下导航折叠 + 表格 overflow
- [x] 480px 以下字号缩小 + 指标网格 2 列
- [x] modal 在小屏幕 `width: calc(100% - 32px)`

### 7. 全量验证
- [x] `npm run build` 构建通过
- [x] `go test ./...` 全部通过（无回归）
- [x] 滚动条 overlay 不占布局空间
