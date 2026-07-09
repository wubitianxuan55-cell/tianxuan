# 系统外观主题双模式重构设计

**日期**: 2026-07-09  
**状态**: 已确认

---

## 1. 动机

当前主题系统存在以下问题：

- 4 个深色主题（dark/warm/ice/forest）仅有深色版本，无对应浅色版本
- `dark` 和 `light` 作为独立"主题"与配色（warm/ice/forest）混在同一层级，概念混淆
- 每种配色仅一套 CSS 变量，缺乏亮暗切换能力

**核心需求**：每种系统配色都同时拥有深色（dark）和浅色（light）两种模式。

---

## 2. 数据模型

### 2.1 两维正交

```
配色方案 (ColorScheme)：default | warm | ice | forest | sunset | ocean | rose | violet
亮暗模式 (ThemeMode)  ：light   | dark | auto
```

`8 × 3 = 24` 种视觉效果。

### 2.2 HTML 属性（CSS/JS 桥梁）

```html
<html data-theme-scheme="warm" data-theme-mode="light">
```

- `data-theme-scheme`：决定强调色和语义色（accent/ok/warn/err）
- `data-theme-mode`：决定结构性颜色（背景层级/文字/边框/阴影）
- `auto` 模式：移除 `data-theme-mode` 属性，CSS 通过 `@media (prefers-color-scheme)` 自动切换

### 2.3 TypeScript 类型

```typescript
export type ColorScheme = "default" | "warm" | "ice" | "forest" | "sunset" | "ocean" | "rose" | "violet";
export type ThemeMode = "light" | "dark" | "auto";
```

### 2.4 localStorage 协议

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `tianxuan-color-scheme` | `ColorScheme` | `"default"` | 配色方案 |
| `tianxuan-theme-mode` | `ThemeMode` | `"dark"` | 亮暗模式 |
| `tianxuan-theme` |（旧，一次性迁移后删除）| — | 旧协议兼容 |

---

## 3. 配色色板

### 3.1 总览

| 配色 | 理念 | 强调色 | 暗色背景基调 | 亮色背景基调 |
|------|------|--------|-------------|-------------|
| default | 专业中性 | 绿(暗)/蓝(亮) | 灰蓝黑 | 灰白 |
| warm | 温暖舒适 | 琥珀金 | 暖棕黑 | 暖奶油 |
| ice | 清冷干净 | 天蓝 | 冰蓝黑 | 极地白 |
| forest | 自然护眼 | 翡翠绿 | 森林黑 | 薄荷白 |
| sunset | 活力热情 | 日落橙 | 暮色紫黑 | 晨光暖白 |
| ocean | 深邃冷静 | 青碧 | 深海藏青 | 海沫蓝白 |
| rose | 优雅柔和 | 玫瑰粉 | 玫紫黑 | 樱花白 |
| violet | 神秘创意 | 紫罗兰 | 暗紫黑 | 薰衣草白 |

### 3.2 逐配色色值

**default 默认**
```
dark:  bg=#0F172A  fg=#E2E8F0  accent=#22C55E  ok=#22C55E  warn=#F59E0B  err=#EF4444
light: bg=#F8FAFC  fg=#1E293B  accent=#3B82F6  ok=#16A34A  warn=#D97706  err=#DC2626
```

**warm 暖色**
```
dark:  bg=#1E1814  fg=#F5E6D3  accent=#F59E0B  ok=#84CC16  warn=#FB923C  err=#F87171
light: bg=#FFFBF5  fg=#3D2E1C  accent=#D97706  ok=#4D7C0F  warn=#EA580C  err=#DC2626
```

**ice 冰蓝**
```
dark:  bg=#0A111A  fg=#E0F2FE  accent=#38BDF8  ok=#34D399  warn=#FBBF24  err=#FCA5A5
light: bg=#F4FAFF  fg=#0C2D48  accent=#0284C7  ok=#059669  warn=#D97706  err=#DC2626
```

**forest 森林**
```
dark:  bg=#0D1510  fg=#DCFCE7  accent=#4ADE80  ok=#4ADE80  warn=#FDE047  err=#FCA5A5
light: bg=#F5FFF8  fg=#1A2E1A  accent=#16A34A  ok=#15803D  warn=#A16207  err=#DC2626
```

**sunset 日落**
```
dark:  bg=#1A1218  fg=#FFF0E6  accent=#F97316  ok=#A3E635  warn=#FBBF24  err=#F87171
light: bg=#FFF8F4  fg=#3D1E0E  accent=#EA580C  ok=#4D7C0F  warn=#B45309  err=#DC2626
```

**ocean 海洋**
```
dark:  bg=#0A1418  fg=#CCFBF1  accent=#14B8A6  ok=#34D399  warn=#FDE047  err=#F87171
light: bg=#F2FFFD  fg=#134E4A  accent=#0D9488  ok=#047857  warn=#B45309  err=#DC2626
```

**rose 玫瑰**
```
dark:  bg=#1A1018  fg=#FCE7F3  accent=#EC4899  ok=#A3E635  warn=#FBBF24  err=#FCA5A5
light: bg=#FFF5FA  fg=#4A1030  accent=#DB2777  ok=#15803D  warn=#B45309  err=#DC2626
```

**violet 紫罗兰**
```
dark:  bg=#14101A  fg=#EDE9FE  accent=#A855F7  ok=#34D399  warn=#FDE047  err=#F87171
light: bg=#F9F7FF  fg=#2E1065  accent=#7C3AED  ok=#047857  warn=#B45309  err=#DC2626
```

---

## 4. CSS 架构

### 4.1 变量分层

```
第 1 层：结构性变量（按 data-theme-mode）
  :root（无 data-theme-mode，即 auto/默认回退）→ 暗色 bg/fg/border/shadow
  [data-theme-mode="dark"]  → 暗色值（显式指定，与 :root 相同）
  [data-theme-mode="light"] → 亮色 bg/fg/border/shadow
  @media (prefers-color-scheme: light) {
    :root:not([data-theme-mode]) → 亮色值（auto 模式下跟随 OS）
  }

第 2 层：配色语义变量（按 data-theme-scheme）
  [data-theme-scheme="default"] → accent/ok/warn/err 默认值
  [data-theme-scheme="warm"]    → accent/ok/warn/err 暖色值
  ...（每个配色一个块，每个约 25 行）

第 3 层：组合覆盖（按需微调，每个约 2-3 行）
  [data-theme-mode="light"][data-theme-scheme="warm"] { ... }
```

### 4.2 CSS 变量清单（不变）

```
--bg, --bg-soft, --bg-elev, --bg-elev-2,
--sidebar-bg, --sidebar-hover, --sidebar-active,
--border, --border-soft,
--fg, --fg-dim, --fg-faint,
--accent, --accent-fg, --accent-soft,
--ok, --warn, --err, --info, --warning, --error, --danger,
--add-bg, --add-fg, --del-bg, --del-fg,
--hl-keyword, --hl-string, --hl-number, --hl-comment,
--hl-func, --hl-type, --hl-builtin, --hl-meta,
--radius, --maxw, --mono, --sans,
--dur-fast, --dur-base, --dur-slow, --dur-slower,
--ease-out, --ease-decelerate, --ease-standard,
--ds-shadow-card, --ds-shadow-card-hover, --ds-shadow-panel,
--ds-shadow-dropdown, --ds-shadow-composer, --ds-shadow-topbar
```

### 4.3 Tailwind 桥接

`tailwind.css` 中 `@theme` 块引用的 CSS 变量名不变，零改动。

---

## 5. UI 交互

### 5.1 设置面板 → 外观标签

双层选择布局：

- **配色方案**：8 个色块卡片（2 行 × 4 列），每个显示 accent 色圆点 + 名称，选中态高亮边框
- **亮暗模式**：三选一分段控件（☀️ 浅色 / 🌙 深色 / 💻 自动）
- 改动即时生效

### 5.2 工具栏 ThemeSwitcher

弹出面板双层选择：

- **配色**：8 个色点（2 行 × 4 列），hover 显示 tooltip 名称
- **模式**：三选一分段控件
- 紧凑布局，节省工具栏空间

### 5.3 预热

`main.tsx` 中 `initTheme()` 同时读取两个 localStorage 键，在 React 渲染前同步设置 `data-theme-scheme` 和 `data-theme-mode`，防止页面闪烁。

---

## 6. 向后兼容

### 6.1 旧主题迁移

旧 `localStorage["tianxuan-theme"]` 值映射：

| 旧值 | → scheme | → mode |
|------|---------|--------|
| `"dark"` | `"default"` | `"dark"` |
| `"light"` | `"default"` | `"light"` |
| `"auto"` | `"default"` | `"auto"` |
| `"warm"` | `"warm"` | `"dark"` |
| `"ice"` | `"ice"` | `"dark"` |
| `"forest"` | `"forest"` | `"dark"` |
| `"midnight"` | `"default"` | `"dark"` |
| `"neon"` | `"default"` | `"dark"` |
| `"mono"` | `"default"` | `"dark"` |
| `"focus"` | `"default"` | `"light"` |
| `"contrast"` | `"default"` | `"dark"` |

迁移后删除 `tianxuan-theme` key。

### 6.2 CSS 向后兼容

- 不再设置 `data-theme` 属性（已废弃）
- 所有 CSS 变量名不变
- `styles.css` 中旧的 `:root[data-theme="..."]` 选择器全部移除
- Tailwind 桥接不变 → 组件零改动

---

## 7. 文件变更清单

| # | 文件 | 操作 | 预估行数 |
|---|------|------|----------|
| 1 | `desktop/frontend/src/lib/theme.ts` | 重写 | ~150 行 |
| 2 | `desktop/frontend/src/styles.css` | 重构 | ~900 行 |
| 3 | `desktop/frontend/src/components/ThemeSwitcher.tsx` | 重写 | ~120 行 |
| 4 | `desktop/frontend/src/components/SettingsAppearance.tsx` | 重写 | ~150 行 |
| 5 | `desktop/frontend/src/hooks/useModeManager.ts` | 改类型 | +5 行 |
| 6 | `desktop/frontend/src/App.tsx` | 改 props | ~3 行 |
| 7 | `desktop/frontend/src/main.tsx` | 改 init | ~2 行 |
| 8 | `desktop/frontend/index.html` | 改初始属性 | ~1 行 |
| 9 | `desktop/frontend/src/locales/zh.ts` | 新增翻译 | +25 行 |
| 10 | `desktop/frontend/src/locales/zh-TW.ts` | 新增翻译 | +25 行 |
| 11 | `desktop/frontend/src/locales/en.ts` | 新增翻译 | +25 行 |

**不改的文件**：`tailwind.css`、`highlight.ts`、`SettingsShared.tsx`、Go 后端、Web 版 `serve/`。
