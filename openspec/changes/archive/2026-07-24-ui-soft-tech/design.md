## Soft Tech 设计语言

### 色彩体系

**暗色主题**
| Token | 值 | 用途 |
|-------|-----|------|
| `--bg-deep` | `#0f0f14` | 页面底色，微暖黑 |
| `--bg-surface` | `#1a1a24` | 卡片/表表面 |
| `--bg-raised` | `#232336` | 悬浮层（modal、dropdown） |
| `--bg-hover` | `#2a2a3d` | 行 hover、按钮 hover |
| `--border` | `#2e2e42` | 边框 |
| `--border-muted` | `#222233` | 弱边框 |
| `--text-primary` | `#e8e8f0` | 主文字 |
| `--text-secondary` | `#9494a8` | 辅助文字 |
| `--text-muted` | `#5c5c72` | 弱文字 |
| `--accent` | `#7c6ff7` | 主强调色（紫蓝） |
| `--accent-dim` | `#5a4fd4` | 深强调色 |
| `--success` | `#34d399` | 成功 |
| `--warn` | `#fbbf24` | 警告 |
| `--danger` | `#f87171` | 危险 |

**亮色主题**
| Token | 值 |
|-------|-----|
| `--bg-deep` | `#f5f5fa` |
| `--bg-surface` | `#ffffff` |
| `--bg-raised` | `#fafafe` |
| `--bg-hover` | `#eef0f8` |
| `--border` | `#e0e2ec` |
| `--border-muted` | `#eef0f6` |
| `--text-primary` | `#1a1a2e` |
| `--text-secondary` | `#6b6b80` |
| `--text-muted` | `#9b9bb0` |
| `--accent` | `#6c5ce7` |
| `--accent-dim` | `#5a4bd1` |

### 组件规范

| 组件 | 规范 |
|------|------|
| **圆角** | 统一 `8px`（卡片）、`6px`（按钮/输入框）、`4px`（徽章） |
| **阴影** | 卡片 `0 1px 3px rgba(0,0,0,0.08)`，模态 `0 8px 32px rgba(0,0,0,0.12)` |
| **间距** | 基于 4px 倍数：4/8/12/16/20/24/32/48/64 |
| **过渡** | 统一 `200ms ease`，hover 背景色和阴影均用此曲线 |
| **边框** | 1px solid，弱场景用 `--border-muted` |
| **字体** | 正文 14px，标题 18px/20px，辅助 12px |

### 滚动条

采用 overlay 模式（不占布局空间）：
```css
* { scrollbar-width: thin; scrollbar-color: var(--text-muted) transparent; }
::-webkit-scrollbar { width: 6px; height: 6px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: var(--text-muted); border-radius: 3px; }
```

### 动效

- 所有 hover 状态：`transition: all 0.2s ease`
- Modal 进出：`opacity + scale`（通过 Vue `<Transition>` 组件）
- 页面切换：保持 `<router-view>` 无动画（SPA 即时切换）

### 响应式断点

- 768px：导航折叠、表格横向滚动
- 480px：指标网格 2 列、字号缩小
