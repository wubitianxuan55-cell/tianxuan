# tianxuan — DESIGN.md

> 单一权威设计文档。所有视觉决策，都从这里出。
> 格式：YAML frontmatter（机器可读） + Markdown 正文（人类可读解释）。

---

## 0. 如何阅读

- **YAML frontmatter** — 精确数值真相源。改设计令牌时，同时更新此文件和 `styles.css`。
- **Markdown 正文** — 设计意图、原则、反模式。新增屏幕前的必读。

两者冲突时，frontmatter 优先。

---

## 1. 项目概况

tianxuan 是基于 Reasonix 的 AI 编程助手桌面端。Wails v2 + React 18 + TailwindCSS 4。
终端风格暗色 UI，单栏聊天布局，侧栏 + 主画布 + Workspace 面板三区。

**设计基调**: 全息科技感 AI 编程助手。深空黑底 + 霓虹青紫渐变，神经节点图标。终端风格保持，但融入未来科技美学。

---

## 2. 设计原则

1. **终端风格为魂** — 暗色为主，等宽字体，极简装饰，功能优先。
2. **一致的令牌系统** — `--ds-*` 前缀是所有视觉决策的单一真相源。
3. **克制使用颜色** — accent 仅用于可操作元素；语义色仅在 status/diff 中使用。
4. **无障碍优先** — 支持 prefers-reduced-motion，32px 最小触摸目标。
5. **性能不妥协** — 无 backdrop-filter（Linux WebKitGTK 兼容），CSS-only 动画。

---

## 3. 颜色令牌

| 令牌 | 暗色 | 亮色 | 用途 |
|------|------|------|------|
| `--ds-accent` | `#6ee7ff` | `#3b82f6` | 主按钮、焦点环、状态点 |
| `--ds-success` | `#74b87a` | `#5a8a5a` | 完成/缓存命中 |
| `--ds-danger` | `#e0696a` | `#c0392b` | 错误/拒绝 |
| `--ds-warning` | `#d9a441` | `#b8860b` | 警告/待处理 |

---

## 4. 圆角阶梯

| 令牌 | 值 | 典型用途 |
|------|-----|---------|
| `--ds-radius-sm` | 6px | 内联代码、kbd |
| `--ds-radius-md` | 8px | 图标按钮 |
| `--ds-radius-lg` | 12px | 卡片（默认） |
| `--ds-radius-xl` | 14px | 对话框/弹窗 |
| `--ds-radius-2xl` | 16px | 大对话框 |
| `--ds-radius-3xl` | 22px | 大面板 |
| `--ds-radius-composer` | 16px | 输入区外壳 |
| `--ds-radius-pill` | 9999px | 胶囊按钮/chip |

规则：可点击表面最小 6px 圆角，卡片永不全圆。

---

## 5. 阴影层级

| 令牌 | 用途 |
|------|------|
| `--ds-shadow-card` | 列表行、侧面板 |
| `--ds-shadow-panel` | 模态框、抽屉 |
| `--ds-shadow-card-hover` | 卡片悬停 |
| `--ds-shadow-dropdown` | 下拉菜单 |
| `--ds-shadow-composer` | 输入区（双层阴影） |
| `--ds-shadow-topbar` | 顶栏 |

规则：所有阴影为黑色/近黑色低 alpha，永不使用彩色阴影。

---

## 6. 动效

| 令牌 | 值 | 用途 |
|------|-----|------|
| `--dur-fast` | 120ms | hover 过渡 |
| `--dur-base` | 180ms | 弹窗/菜单 |
| `--dur-slow` | 340ms | 抽屉/面板 |
| `--dur-slower` | 420ms | 大叠层淡出 |

全局 `prefers-reduced-motion` 支持。

---

## 7. 组件配方

### 主按钮 (Primary)
```
inline-flex items-center gap-1.5 rounded-full bg-accent px-4 py-2
text-[13px] font-semibold text-accent-fg transition hover:brightness-110
active:scale-[0.97]
```

### 次按钮 (Secondary)
```
inline-flex items-center gap-1.5 rounded-lg border border-border
bg-bg-elev px-3 py-2 text-[13px] font-medium text-fg
transition hover:bg-bg-soft disabled:opacity-50
```

### 卡片 (Card)
```
border border-border bg-bg-elev rounded-xl
box-shadow: var(--ds-shadow-card)
```

### Chip
```
inline-flex items-center gap-1 rounded-full bg-accent-soft
px-1.5 py-0.5 text-[10px] font-semibold text-accent
```

---

## 8. 禁止事项

- 不使用 emoji 作为功能性 UI 元素
- 不在非可点击表面使用 accent 色
- 不对卡片使用 `rounded-full`
- 不使用小于 6px 的圆角
- 不添加 backdrop-filter / blur（Linux 兼容）
- 不使用彩色阴影
