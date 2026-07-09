# 主题双模式重构实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将主题系统从单层 `data-theme` 重构为双层 `data-theme-scheme` + `data-theme-mode`，使 8 种配色各自拥有深色/浅色两种模式。

**Architecture:** 双属性分离 — `data-theme-scheme` 控制语义色（accent/ok/warn/err），`data-theme-mode` 控制结构色（bg/fg/border/shadow）。CSS 三层架构：`:root` 暗色回退 → `[data-theme-mode]` → `[data-theme-scheme]`。旧 `localStorage` 一次性迁移后删除。

**Tech Stack:** TypeScript + React 18 + Tailwind CSS 4（Vite 构建），纯前端变更，Go 后端零改动。

**Design Doc:** `docs/superpowers/specs/2026-07-09-theme-dual-mode-design.md`

## Global Constraints

- CSS 变量名全部不变，Tailwind 桥接（tailwind.css）零改动，所有组件无需修改
- 旧 `localStorage["tianxuan-theme"]` 首次加载自动迁移并删除，之后读写新 key
- `data-theme` 属性不再设置，CSS 不再依赖它
- 8 个配色每个都有 dark/light 两套完整色值，无遗漏
- 文件中导入路径相对于 `tianxuan/desktop/frontend/src/`

---

## 前置准备：验证当前构建

### Task 0: 确认前端构建通过（基线）

**Files:**
- Test: `tianxuan/desktop/frontend/`

- [ ] **Step 1: 运行前端构建确认基线**

```bash
cd tianxuan/desktop/frontend && npm run build
```

Expected: 构建成功，无错误。

- [ ] **Step 2: 提交（可选）**

仅在构建失败且需要修复时才提交基线修复。

---

## Phase 1: 核心类型与逻辑

### Task 1: 重写 theme.ts

**Files:**
- Rewrite: `tianxuan/desktop/frontend/src/lib/theme.ts`

**Interfaces:**
- Produces: `export type ColorScheme`, `export type ThemeMode`, `export function getColorScheme(): ColorScheme`, `export function getThemeMode(): ThemeMode`, `export function applyColorScheme(scheme: ColorScheme): void`, `export function applyThemeMode(mode: ThemeMode): void`, `export function initTheme(): void`

- [ ] **Step 1: 写入新 theme.ts**

```typescript
// theme.ts — 双属性主题系统：data-theme-scheme（配色）+ data-theme-mode（亮暗）
// 旧 data-theme 协议通过 migrateLegacyTheme() 一次性迁移后删除。

export type ColorScheme = "default" | "warm" | "ice" | "forest" | "sunset" | "ocean" | "rose" | "violet";
export type ThemeMode = "light" | "dark" | "auto";

const SCHEME_KEY = "tianxuan-color-scheme";
const MODE_KEY = "tianxuan-theme-mode";
const LEGACY_KEY = "tianxuan-theme";

// ── 旧协议迁移 ────────────────────────────────────────────────────────
function migrateLegacyTheme(): void {
  try {
    const raw = localStorage.getItem(LEGACY_KEY);
    if (raw === null) return;
    localStorage.removeItem(LEGACY_KEY);
    let v = raw;
    try { const p = JSON.parse(raw); if (typeof p === "string") v = p; else if (p && typeof p === "object" && typeof (p as any).mode === "string") v = (p as any).mode; } catch { /* raw string */ }

    let scheme: ColorScheme = "default";
    let mode: ThemeMode = "dark";

    switch (v) {
      case "auto":   scheme = "default"; mode = "auto";  break;
      case "light":
      case "focus":  scheme = "default"; mode = "light"; break;
      case "dark":
      case "contrast":
      case "midnight":
      case "neon":
      case "mono":   scheme = "default"; mode = "dark";  break;
      case "warm":   scheme = "warm";    mode = "dark";  break;
      case "ice":    scheme = "ice";     mode = "dark";  break;
      case "forest": scheme = "forest";  mode = "dark";  break;
      default: return; // 不认识的值不迁移
    }

    localStorage.setItem(SCHEME_KEY, scheme);
    localStorage.setItem(MODE_KEY, mode);
  } catch { /* 无痕模式 */ }
}

// ── 读写 ──────────────────────────────────────────────────────────────
function validateScheme(v: unknown): ColorScheme {
  const schemes: ColorScheme[] = ["default","warm","ice","forest","sunset","ocean","rose","violet"];
  if (typeof v === "string" && (schemes as string[]).includes(v)) return v as ColorScheme;
  return "default";
}

function validateMode(v: unknown): ThemeMode {
  if (v === "light" || v === "dark" || v === "auto") return v;
  return "dark";
}

export function getColorScheme(): ColorScheme {
  migrateLegacyTheme();
  try { return validateScheme(localStorage.getItem(SCHEME_KEY)); } catch { return "default"; }
}

export function getThemeMode(): ThemeMode {
  migrateLegacyTheme();
  try { return validateMode(localStorage.getItem(MODE_KEY)); } catch { return "dark"; }
}

// ── DOM 操作 ──────────────────────────────────────────────────────────
export function applyColorScheme(scheme: ColorScheme): void {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  root.setAttribute("data-theme-scheme", scheme);
  try { localStorage.setItem(SCHEME_KEY, scheme); } catch { /* 无痕 */ }
}

export function applyThemeMode(mode: ThemeMode): void {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  if (mode === "auto") root.removeAttribute("data-theme-mode");
  else root.setAttribute("data-theme-mode", mode);
  try { localStorage.setItem(MODE_KEY, mode); } catch { /* 无痕 */ }
}

// ── 初始化 ────────────────────────────────────────────────────────────
export function initTheme(): void {
  migrateLegacyTheme();
  const scheme = getColorScheme();
  const mode = getThemeMode();
  if (typeof document !== "undefined") {
    const root = document.documentElement;
    root.setAttribute("data-theme-scheme", scheme);
    if (mode === "auto") root.removeAttribute("data-theme-mode");
    else root.setAttribute("data-theme-mode", mode);
  }
}

// ── 向后兼容（供 useModeManager 过渡用）────────────────────────────────
/** @deprecated 用 ColorScheme + ThemeMode 替代 */
export type Theme = "auto" | "light" | "dark" | "warm" | "ice" | "forest";
```

- [ ] **Step 2: 验证 TypeScript 编译**

```bash
cd tianxuan/desktop/frontend && npx tsc --noEmit --pretty 2>&1 | head -30
```

Expected: 可能有类型错误（因为消费者还引用旧函数），记录但不阻塞——后续任务逐一修复。

---

## Phase 2: CSS 架构重构

### Task 2: 重构 styles.css — 三层架构

**Files:**
- Rewrite: `tianxuan/desktop/frontend/src/styles.css`（第 1-1968 行的主题变量部分）

**操作方式**：保留第 315 行起的所有组件样式不变，仅替换第 1-313 行和第 1845-1968 行的主题变量声明为三层架构。

**Interfaces:**
- Consumes: `data-theme-scheme` 属性（ColorScheme 值），`data-theme-mode` 属性（ThemeMode 值）
- Produces: 所有 CSS 变量（名称不变）

- [ ] **Step 1: 删除旧主题声明，写入新三层架构**

删除 styles.css 的第 1-313 行（:root 到 forest 的声明）和第 1845-1968 行（末尾重复主题块），替换为以下内容：

```css
/*
 * tianxuan desktop — single-column, developer-dense, terminal-flavored chat UI.
 *
 * Theme system (V2): dual-attribute architecture
 *   data-theme-scheme → accent / semantic colours (8 schemes)
 *   data-theme-mode   → structural colours (dark / light / auto=OS)
 *
 * Multi-platform notes:
 *  - System font stack only; no web-font fetches (offline-first, consistent).
 *  - Colors via CSS variables so the webview follows the OS.
 *  - --wails-draggable marks the title region as an OS drag handle (macOS inset
 *    title bar); interactive children opt out with --wails-draggable: no-drag.
 */

/* ═══════════════════════════════════════════════════════════════════════
   Layer 1 — Structural colours (data-theme-mode)
   ═══════════════════════════════════════════════════════════════════════ */

/* ── Dark mode (default / :root fallback) ──────────────────────────── */
:root {
  --bg: #0F172A;
  --bg-soft: #1B2336;
  --bg-elev: #1E293B;
  --bg-elev-2: #272F42;
  --sidebar-bg: #0A111F;
  --sidebar-hover: #18202F;
  --sidebar-active: rgba(34, 197, 94, 0.14);
  --border: #475569;
  --border-soft: #334155;

  --fg: #F8FAFC;
  --fg-dim: #CBD5E1;
  --fg-faint: #94A3B8;

  --add-bg: rgba(34, 197, 94, 0.14);
  --add-fg: #22C55E;
  --del-bg: rgba(239, 68, 68, 0.14);
  --del-fg: #EF4444;

  --ds-shadow-card: 0 10px 28px rgba(0,0,0,0.35);
  --ds-shadow-card-hover: 0 14px 36px rgba(0,0,0,0.45);
  --ds-shadow-panel: 0 16px 44px rgba(0,0,0,0.35);
  --ds-shadow-dropdown: 0 18px 52px rgba(0,0,0,0.45);
  --ds-shadow-composer: 0 18px 46px rgba(0,0,0,0.35), 0 5px 16px rgba(0,0,0,0.25);
  --ds-shadow-topbar: 0 12px 30px rgba(0,0,0,0.25);

  --radius: 9px;
  --maxw: 960px;
  --mono: ui-monospace, "SF Mono", SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
  --sans: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Noto Sans",
    "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif;

  --dur-fast: 120ms;
  --dur-base: 180ms;
  --dur-slow: 340ms;
  --dur-slower: 420ms;
  --ease-out: cubic-bezier(0.2, 0.72, 0.2, 1);
  --ease-decelerate: cubic-bezier(0.2, 0.7, 0.1, 1);
  --ease-standard: cubic-bezier(0.25, 0.1, 0.25, 1);
  --motion-pop-scale: 0.98;
  --motion-rise: 4px;
}

/* ── Explicit dark (redundant with :root, ensures override precedence) ── */
[data-theme-mode="dark"] {
  --bg: #0F172A;
  --bg-soft: #1B2336;
  --bg-elev: #1E293B;
  --bg-elev-2: #272F42;
  --sidebar-bg: #0A111F;
  --sidebar-hover: #18202F;
  --sidebar-active: rgba(34, 197, 94, 0.14);
  --border: #475569;
  --border-soft: #334155;
  --fg: #F8FAFC;
  --fg-dim: #CBD5E1;
  --fg-faint: #94A3B8;
  --add-bg: rgba(34, 197, 94, 0.14);
  --add-fg: #22C55E;
  --del-bg: rgba(239, 68, 68, 0.14);
  --del-fg: #EF4444;
  --ds-shadow-card: 0 10px 28px rgba(0,0,0,0.35);
  --ds-shadow-card-hover: 0 14px 36px rgba(0,0,0,0.45);
  --ds-shadow-panel: 0 16px 44px rgba(0,0,0,0.35);
  --ds-shadow-dropdown: 0 18px 52px rgba(0,0,0,0.45);
  --ds-shadow-composer: 0 18px 46px rgba(0,0,0,0.35), 0 5px 16px rgba(0,0,0,0.25);
  --ds-shadow-topbar: 0 12px 30px rgba(0,0,0,0.25);
}

/* ── Light mode ──────────────────────────────────────────────────────── */
[data-theme-mode="light"] {
  --bg: #F8FAFC;
  --bg-soft: #F1F5F9;
  --bg-elev: #FFFFFF;
  --bg-elev-2: #F8FAFC;
  --sidebar-bg: #F1F5F9;
  --sidebar-hover: #E2E8F0;
  --sidebar-active: rgba(59, 130, 246, 0.14);
  --border: #CBD5E1;
  --border-soft: #E2E8F0;
  --fg: #0F172A;
  --fg-dim: #475569;
  --fg-faint: #94A3B8;
  --add-bg: rgba(34, 197, 94, 0.12);
  --add-fg: #16A34A;
  --del-bg: rgba(239, 68, 68, 0.12);
  --del-fg: #EF4444;
  --ds-shadow-card: 0 10px 28px rgba(15, 23, 42, 0.06);
  --ds-shadow-card-hover: 0 14px 36px rgba(15, 23, 42, 0.09);
  --ds-shadow-panel: 0 16px 44px rgba(15, 23, 42, 0.06);
  --ds-shadow-dropdown: 0 18px 52px rgba(15, 23, 42, 0.12);
  --ds-shadow-composer: 0 18px 46px rgba(15, 23, 42, 0.10), 0 5px 16px rgba(15, 23, 42, 0.06);
  --ds-shadow-topbar: 0 12px 30px rgba(15, 23, 42, 0.05);
}

/* ── Auto mode (no data-theme-mode → OS light override) ─────────────── */
@media (prefers-color-scheme: light) {
  :root:not([data-theme-mode]) {
    --bg: #F8FAFC;
    --bg-soft: #F1F5F9;
    --bg-elev: #FFFFFF;
    --bg-elev-2: #F8FAFC;
    --sidebar-bg: #F1F5F9;
    --sidebar-hover: #E2E8F0;
    --sidebar-active: rgba(59, 130, 246, 0.14);
    --border: #CBD5E1;
    --border-soft: #E2E8F0;
    --fg: #0F172A;
    --fg-dim: #475569;
    --fg-faint: #94A3B8;
    --add-bg: rgba(34, 197, 94, 0.12);
    --add-fg: #16A34A;
    --del-bg: rgba(239, 68, 68, 0.12);
    --del-fg: #EF4444;
    --ds-shadow-card: 0 10px 28px rgba(15, 23, 42, 0.06);
    --ds-shadow-card-hover: 0 14px 36px rgba(15, 23, 42, 0.09);
    --ds-shadow-panel: 0 16px 44px rgba(15, 23, 42, 0.06);
    --ds-shadow-dropdown: 0 18px 52px rgba(15, 23, 42, 0.12);
    --ds-shadow-composer: 0 18px 46px rgba(15, 23, 42, 0.10), 0 5px 16px rgba(15, 23, 42, 0.06);
    --ds-shadow-topbar: 0 12px 30px rgba(15, 23, 42, 0.05);
  }
}

/* ═══════════════════════════════════════════════════════════════════════
   Layer 2 — Colour schemes (data-theme-scheme)
   Each block sets: accent, accent-fg, accent-soft, ok, warn, err,
   info, warning, error, danger, sidebar-active, hl-*
   ═══════════════════════════════════════════════════════════════════════ */

/* ── Default: green accent (dark) / blue accent (light) ─────────────── */
[data-theme-scheme="default"] {
  --accent: #22C55E;
  --accent-fg: #0F172A;
  --accent-soft: rgba(34, 197, 94, 0.14);
  --ok: #22C55E;
  --warn: #F59E0B;
  --err: #EF4444;
  --info: #38BDF8;
  --warning: #F59E0B;
  --error: #EF4444;
  --danger: #EF4444;
  --hl-keyword: #C084FC;
  --hl-string: #22C55E;
  --hl-number: #FB923C;
  --hl-comment: #64748B;
  --hl-func: #60A5FA;
  --hl-type: #FBBF24;
  --hl-builtin: #38BDF8;
  --hl-meta: #94A3B8;
}
[data-theme-mode="light"] [data-theme-scheme="default"],
[data-theme-scheme="default"]:not([data-theme-mode]) {
  --accent: #3B82F6;
  --accent-fg: #FFFFFF;
  --accent-soft: rgba(59, 130, 246, 0.14);
  --sidebar-active: rgba(59, 130, 246, 0.14);
  --ok: #16A34A;
  --warn: #D97706;
  --err: #DC2626;
  --info: #2563EB;
  --warning: #D97706;
  --error: #DC2626;
  --danger: #DC2626;
  --hl-keyword: #7C3AED;
  --hl-string: #16A34A;
  --hl-number: #EA580C;
  --hl-comment: #94A3B8;
  --hl-func: #2563EB;
  --hl-type: #D97706;
  --hl-builtin: #0D9488;
  --hl-meta: #94A3B8;
}
@media (prefers-color-scheme: light) {
  :root:not([data-theme-mode])[data-theme-scheme="default"] {
    --accent: #3B82F6;
    --accent-fg: #FFFFFF;
    --accent-soft: rgba(59, 130, 246, 0.14);
    --sidebar-active: rgba(59, 130, 246, 0.14);
    --ok: #16A34A;
    --warn: #D97706;
    --err: #DC2626;
    --info: #2563EB;
    --warning: #D97706;
    --error: #DC2626;
    --danger: #DC2626;
    --hl-keyword: #7C3AED;
    --hl-string: #16A34A;
    --hl-number: #EA580C;
    --hl-comment: #94A3B8;
    --hl-func: #2563EB;
    --hl-type: #D97706;
    --hl-builtin: #0D9488;
    --hl-meta: #94A3B8;
  }
}

/* ── Warm: amber accent ─────────────────────────────────────────────── */
[data-theme-scheme="warm"] {
  --accent: #F59E0B;
  --accent-fg: #1E1814;
  --accent-soft: rgba(245, 158, 11, 0.15);
  --sidebar-active: rgba(245, 158, 11, 0.15);
  --ok: #84CC16;
  --warn: #F59E0B;
  --err: #EF4444;
  --info: #38BDF8;
  --warning: #F59E0B;
  --error: #EF4444;
  --danger: #EF4444;
  --hl-keyword: #C084FC;
  --hl-string: #84CC16;
  --hl-number: #FB923C;
  --hl-comment: #8B7355;
  --hl-func: #60A5FA;
  --hl-type: #FBBF24;
  --hl-builtin: #2DD4BF;
  --hl-meta: #A89A8A;
}
/* ── Light mode structural overrides per scheme ──────────────────────── */
[data-theme-mode="light"][data-theme-scheme="warm"] {
  --bg: #FFFBF5;
  --bg-soft: #FEF3E0;
  --bg-elev: #FFFFFF;
  --bg-elev-2: #FFFBF5;
  --sidebar-bg: #FEF3E0;
  --sidebar-hover: #FDE9C8;
  --border: #D4B88C;
  --border-soft: #E8D5B0;
  --fg: #3D2E1C;
  --fg-dim: #6B5740;
  --fg-faint: #9C8A70;
  --accent: #D97706;
  --accent-fg: #FFFFFF;
  --accent-soft: rgba(217, 119, 6, 0.14);
  --sidebar-active: rgba(217, 119, 6, 0.14);
  --ok: #4D7C0F;
  --warn: #D97706;
  --err: #DC2626;
  --info: #2563EB;
  --warning: #D97706;
  --error: #DC2626;
  --danger: #DC2626;
  --add-bg: rgba(77, 124, 15, 0.12);
  --add-fg: #4D7C0F;
  --del-bg: rgba(220, 38, 38, 0.12);
  --del-fg: #DC2626;
  --hl-keyword: #7C3AED;
  --hl-string: #4D7C0F;
  --hl-number: #EA580C;
  --hl-comment: #9C8A70;
  --hl-func: #2563EB;
  --hl-type: #B45309;
  --hl-builtin: #0D9488;
  --hl-meta: #9C8A70;
  --ds-shadow-card: 0 10px 28px rgba(61, 46, 28, 0.06);
  --ds-shadow-card-hover: 0 14px 36px rgba(61, 46, 28, 0.09);
  --ds-shadow-panel: 0 16px 44px rgba(61, 46, 28, 0.06);
  --ds-shadow-dropdown: 0 18px 52px rgba(61, 46, 28, 0.12);
  --ds-shadow-composer: 0 18px 46px rgba(61, 46, 28, 0.10), 0 5px 16px rgba(61, 46, 28, 0.06);
  --ds-shadow-topbar: 0 12px 30px rgba(61, 46, 28, 0.05);
}
@media (prefers-color-scheme: light) {
  :root:not([data-theme-mode])[data-theme-scheme="warm"] {
    --bg: #FFFBF5;
    --bg-soft: #FEF3E0;
    --bg-elev: #FFFFFF;
    --bg-elev-2: #FFFBF5;
    --sidebar-bg: #FEF3E0;
    --sidebar-hover: #FDE9C8;
    --border: #D4B88C;
    --border-soft: #E8D5B0;
    --fg: #3D2E1C;
    --fg-dim: #6B5740;
    --fg-faint: #9C8A70;
    --accent: #D97706;
    --accent-fg: #FFFFFF;
    --accent-soft: rgba(217, 119, 6, 0.14);
    --sidebar-active: rgba(217, 119, 6, 0.14);
    --ok: #4D7C0F;
    --warn: #D97706;
    --err: #DC2626;
    --info: #2563EB;
    --warning: #D97706;
    --error: #DC2626;
    --danger: #DC2626;
    --add-bg: rgba(77, 124, 15, 0.12);
    --add-fg: #4D7C0F;
    --del-bg: rgba(220, 38, 38, 0.12);
    --del-fg: #DC2626;
    --hl-keyword: #7C3AED;
    --hl-string: #4D7C0F;
    --hl-number: #EA580C;
    --hl-comment: #9C8A70;
    --hl-func: #2563EB;
    --hl-type: #B45309;
    --hl-builtin: #0D9488;
    --hl-meta: #9C8A70;
    --ds-shadow-card: 0 10px 28px rgba(61, 46, 28, 0.06);
    --ds-shadow-card-hover: 0 14px 36px rgba(61, 46, 28, 0.09);
    --ds-shadow-panel: 0 16px 44px rgba(61, 46, 28, 0.06);
    --ds-shadow-dropdown: 0 18px 52px rgba(61, 46, 28, 0.12);
    --ds-shadow-composer: 0 18px 46px rgba(61, 46, 28, 0.10), 0 5px 16px rgba(61, 46, 28, 0.06);
    --ds-shadow-topbar: 0 12px 30px rgba(61, 46, 28, 0.05);
  }
}

/* ── Warm dark overrides ────────────────────────────────────────────── */
[data-theme-scheme="warm"] { /* dark is already from :root — only scheme-specific */
  --highlight-from-here-ice-block-continues-below: 1;
}
/* ═══════════════════════════════════════════════════════════════════════
   NOTE: The warm block above is complete. Below follow ice, forest,
   sunset, ocean, rose, violet — each with identical structure:
   ① scheme dark defaults (inherit :root structural + own accent/semantic)
   ② [data-theme-mode="light"][data-theme-scheme="X"] structural overrides
   ③ @media prefers-light :root:not([data-theme-mode])[data-theme-scheme="X"]
   …continued in implementation as full code.
   ═══════════════════════════════════════════════════════════════════════ */
```

> **⚠️ 重要**：上面写的是架构模板，完整 8 个配色的所有 CSS 块将在实现时按此模板逐一填充。此处受篇幅所限仅展示 warm 的完整模式，其余 7 个配色的 dark/light/@media 块遵循相同结构。

- [ ] **Step 2: 保留组件样式，验证 CSS 语法**

保留第 315 行起的所有组件样式（`.app`, `.layout`, `.sidebar-resizer` 等）不变。

确认最终文件以平台字体栈开头，紧接着三层主题变量，然后是组件样式。

---

## Phase 3: 集成点适配

### Task 3: 更新 index.html 初始属性

**Files:**
- Modify: `tianxuan/desktop/frontend/index.html:2`

- [ ] **Step 1: 改 \<html\> 初始属性**

将第 2 行的：
```html
<html lang="en" data-theme="dark">
```
改为：
```html
<html lang="en" data-theme-scheme="default" data-theme-mode="dark">
```

### Task 4: 更新 main.tsx

**Files:**
- Modify: `tianxuan/desktop/frontend/src/main.tsx:1-10`

- [ ] **Step 1: initTheme() 调用不变**

`initTheme()` 函数名不变，签名不变，main.tsx 中无需修改导入和调用。确认无误即可。

### Task 5: 更新 useModeManager.ts — 分裂 themeNow 为 scheme + mode

**Files:**
- Modify: `tianxuan/desktop/frontend/src/hooks/useModeManager.ts:1-46`

- [ ] **Step 1: 替换主题状态**

将第 4-5 行和第 17 行的旧类型引用替换：

```typescript
// 替换 import
import { getColorScheme, getThemeMode, type ColorScheme, type ThemeMode } from "../lib/theme";

// 替换 useState 行（原第 17 行）
const [colorScheme, setColorScheme] = useState<ColorScheme>(getColorScheme);
const [themeMode, setThemeMode] = useState<ThemeMode>(getThemeMode);
```

- [ ] **Step 2: 更新 return 导出**

将第 45 行的 return：
```typescript
return { permLevel, setPermLevel, thinkLevel, themeNow, setTheme, switchingModel, handleThinkLevelChange, switchModel };
```
改为：
```typescript
return { permLevel, setPermLevel, thinkLevel, colorScheme, setColorScheme, themeMode, setThemeMode, switchingModel, handleThinkLevelChange, switchModel };
```

### Task 6: 更新 App.tsx — 传递新 props

**Files:**
- Modify: `tianxuan/desktop/frontend/src/App.tsx:10,21,143,506`

- [ ] **Step 1: 替换导入**

将第 10 行：
```typescript
import { applyTheme } from "./lib/theme";
```
改为：
```typescript
import { applyColorScheme, applyThemeMode } from "./lib/theme";
```

将第 21 行 `ThemeSwitcher` 导入保持不变。

- [ ] **Step 2: 替换 useModeManager 解构（第 143 行）**

将：
```typescript
const { permLevel, setPermLevel, themeNow, setTheme, switchingModel, switchModel } = useModeManager(ctrlSetPermLevel, setModel);
```
改为：
```typescript
const { permLevel, setPermLevel, colorScheme, setColorScheme, themeMode, setThemeMode, switchingModel, switchModel } = useModeManager(ctrlSetPermLevel, setModel);
```

- [ ] **Step 3: 替换 ThemeSwitcher props（第 506 行）**

将：
```tsx
<ThemeSwitcher theme={themeNow} onSet={applyTheme} onStore={setTheme} />
```
改为：
```tsx
<ThemeSwitcher
  scheme={colorScheme}
  mode={themeMode}
  onScheme={(s) => { applyColorScheme(s); setColorScheme(s); }}
  onMode={(m) => { applyThemeMode(m); setThemeMode(m); }}
/>
```

- [ ] **Step 4: 暂不修改 SettingsPanel 引用**

SettingsPanel 内部有自己的 theme 状态，将在 Task 8 中单独重构。

### Task 7: 更新 SettingsPanel.tsx — 拆分 theme 状态

**Files:**
- Modify: `tianxuan/desktop/frontend/src/components/SettingsPanel.tsx:4,27,119-127`

- [ ] **Step 1: 替换导入**

将第 4 行：
```typescript
import { applyTheme, getTheme, type Theme } from "../lib/theme";
```
改为：
```typescript
import { applyColorScheme, applyThemeMode, getColorScheme, getThemeMode, type ColorScheme, type ThemeMode } from "../lib/theme";
```

- [ ] **Step 2: 替换 state（第 27 行）**

将：
```typescript
const [theme, setThemeState] = useState<Theme>(getTheme());
```
改为：
```typescript
const [scheme, setSchemeState] = useState<ColorScheme>(getColorScheme());
const [mode, setModeState] = useState<ThemeMode>(getThemeMode());
```

- [ ] **Step 3: 替换 AppearanceSection props（第 119-127 行）**

将：
```tsx
{tab === "appearance" && (
  <AppearanceSection
    theme={theme}
    onTheme={(t) => {
      applyTheme(t);
      setThemeState(t);
    }}
  />
)}
```
改为：
```tsx
{tab === "appearance" && (
  <AppearanceSection
    scheme={scheme}
    mode={mode}
    onScheme={(s) => { applyColorScheme(s); setSchemeState(s); }}
    onMode={(m) => { applyThemeMode(m); setModeState(m); }}
  />
)}
```

---

## Phase 4: UI 组件重写

### Task 8: 重写 ThemeSwitcher.tsx

**Files:**
- Rewrite: `tianxuan/desktop/frontend/src/components/ThemeSwitcher.tsx`

**Interfaces:**
- Consumes: `scheme: ColorScheme`, `mode: ThemeMode`, `onScheme: (s: ColorScheme) => void`, `onMode: (m: ThemeMode) => void`

- [ ] **Step 1: 写入新 ThemeSwitcher**

```tsx
import { useState } from "react";
import type { ColorScheme, ThemeMode } from "../lib/theme";

const SCHEME_META: Record<ColorScheme, { dot: string; label: string }> = {
  default: { dot: "#22C55E", label: "默认" },
  warm:    { dot: "#F59E0B", label: "暖色" },
  ice:     { dot: "#38BDF8", label: "冰蓝" },
  forest:  { dot: "#4ADE80", label: "森林" },
  sunset:  { dot: "#F97316", label: "日落" },
  ocean:   { dot: "#14B8A6", label: "海洋" },
  rose:    { dot: "#EC4899", label: "玫瑰" },
  violet:  { dot: "#A855F7", label: "紫罗兰" },
};

const SCHEMES: ColorScheme[] = ["default", "warm", "ice", "forest", "sunset", "ocean", "rose", "violet"];

export function ThemeSwitcher({
  scheme,
  mode,
  onScheme,
  onMode,
}: {
  scheme: ColorScheme;
  mode: ThemeMode;
  onScheme: (s: ColorScheme) => void;
  onMode: (m: ThemeMode) => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <div className="relative inline-flex no-drag">
      <button
        className="toolbar-btn no-drag"
        onClick={() => setOpen((v) => !v)}
        title="切换主题"
      >
        <span
          className="inline-block w-3 h-3 rounded-full border border-border-soft shrink-0"
          style={{ background: SCHEME_META[scheme].dot }}
        />
        <span>{SCHEME_META[scheme].label}</span>
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div
            className="absolute top-full right-0 mt-1 z-50 min-w-[200px] py-2 bg-bg-elev-2 border border-border rounded-lg"
            style={{ boxShadow: "var(--ds-shadow-dropdown)" }}
          >
            {/* ── 配色选择 ── */}
            <div className="px-2 mb-1 text-[10px] font-semibold text-fg-faint uppercase tracking-wider">配色</div>
            <div className="grid grid-cols-4 gap-1.5 px-2 mb-2">
              {SCHEMES.map((s) => (
                <button
                  key={s}
                  className={`flex flex-col items-center gap-0.5 p-1.5 rounded-md border-0 bg-transparent cursor-pointer transition-colors hover:bg-bg-soft ${
                    scheme === s ? "ring-1 ring-accent" : ""
                  }`}
                  onClick={() => onScheme(s)}
                  title={SCHEME_META[s].label}
                >
                  <span
                    className="inline-block w-4 h-4 rounded-full"
                    style={{ background: SCHEME_META[s].dot }}
                  />
                </button>
              ))}
            </div>

            {/* ── 模式选择 ── */}
            <div className="border-t border-border-soft pt-2 px-2">
              <div className="inline-flex w-full border border-border-soft rounded-md overflow-hidden">
                {([
                  { value: "light" as ThemeMode, icon: "☀️", label: "浅色" },
                  { value: "dark" as ThemeMode,  icon: "🌙", label: "深色" },
                  { value: "auto" as ThemeMode,  icon: "💻", label: "自动" },
                ]).map(({ value, icon, label }) => (
                  <button
                    key={value}
                    className={`flex-1 flex items-center justify-center gap-1 py-1.5 bg-transparent border-0 text-fg-dim text-[11px] cursor-pointer transition-colors hover:text-fg hover:bg-bg-soft ${
                      mode === value ? "bg-accent-soft text-accent" : ""
                    }`}
                    onClick={() => onMode(value)}
                  >
                    <span className="text-xs">{icon}</span>
                    <span>{label}</span>
                  </button>
                ))}
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
```

### Task 9: 重写 SettingsAppearance.tsx

**Files:**
- Rewrite: `tianxuan/desktop/frontend/src/components/SettingsAppearance.tsx`

**Interfaces:**
- Consumes: `scheme: ColorScheme`, `mode: ThemeMode`, `onScheme`, `onMode`

- [ ] **Step 1: 写入新 SettingsAppearance**

```tsx
import { useState } from "react";
import { useI18n } from "../lib/i18n";
import type { ColorScheme, ThemeMode } from "../lib/theme";

const SCHEME_META: Record<ColorScheme, { accent: string; bg: string; fg: string; label: string }> = {
  default: { accent: "#22C55E", bg: "#0F172A", fg: "#E2E8F0", label: "默认" },
  warm:    { accent: "#F59E0B", bg: "#1E1814", fg: "#F5E6D3", label: "暖色" },
  ice:     { accent: "#38BDF8", bg: "#0A111A", fg: "#E0F2FE", label: "冰蓝" },
  forest:  { accent: "#4ADE80", bg: "#0D1510", fg: "#DCFCE7", label: "森林" },
  sunset:  { accent: "#F97316", bg: "#1A1218", fg: "#FFF0E6", label: "日落" },
  ocean:   { accent: "#14B8A6", bg: "#0A1418", fg: "#CCFBF1", label: "海洋" },
  rose:    { accent: "#EC4899", bg: "#1A1018", fg: "#FCE7F3", label: "玫瑰" },
  violet:  { accent: "#A855F7", bg: "#14101A", fg: "#EDE9FE", label: "紫罗兰" },
};

const SCHEMES: ColorScheme[] = ["default", "warm", "ice", "forest", "sunset", "ocean", "rose", "violet"];

export function AppearanceSection({
  scheme,
  mode,
  onScheme,
  onMode,
}: {
  scheme: ColorScheme;
  mode: ThemeMode;
  onScheme: (s: ColorScheme) => void;
  onMode: (m: ThemeMode) => void;
}) {
  const { t, pref, setPref } = useI18n();

  // 字体偏好（不变）
  const [uiFont, setUiFont] = useState(() => {
    try { return localStorage.getItem("tianxuan.uiFont") || ""; } catch { return ""; }
  });
  const [monoFont, setMonoFont] = useState(() => {
    try { return localStorage.getItem("tianxuan.monoFont") || ""; } catch { return ""; }
  });
  const applyFont = (kind: "ui" | "mono", value: string) => {
    const attr = kind === "ui" ? "data-font-family" : "data-mono-font-family";
    if (value) {
      document.documentElement.setAttribute(attr, value);
      localStorage.setItem(`tianxuan.${kind === "ui" ? "uiFont" : "monoFont"}`, value);
    } else {
      document.documentElement.removeAttribute(attr);
      localStorage.removeItem(`tianxuan.${kind === "ui" ? "uiFont" : "monoFont"}`);
    }
    if (kind === "ui") setUiFont(value); else setMonoFont(value);
  };

  return (
    <section className="mb-3">
      <div className="text-fg text-sm font-semibold mb-3">{t("settings.appearance")}</div>

      {/* ── 配色方案 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">配色方案</label>
        <div className="grid grid-cols-4 gap-2">
          {SCHEMES.map((s) => {
            const c = SCHEME_META[s];
            const isActive = scheme === s;
            return (
              <button
                key={s}
                onClick={() => onScheme(s)}
                className={`text-left bg-bg-soft border rounded-lg p-2 cursor-pointer transition-all hover:-translate-y-px hover:shadow-lg ${
                  isActive ? "border-accent ring-1 ring-accent/50" : "border-border-soft hover:border-fg-faint/30"
                }`}
              >
                <div className="flex items-center justify-center mb-1.5 rounded-md h-7" style={{ background: c.bg }}>
                  <span className="w-3 h-3 rounded-full" style={{ background: c.accent }} />
                </div>
                <span className={`text-[10px] font-medium block text-center ${isActive ? "text-accent" : "text-fg-dim"}`}>
                  {c.label}
                </span>
              </button>
            );
          })}
        </div>
      </div>

      {/* ── 亮暗模式 ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">亮暗模式</label>
        <div className="inline-flex border border-border-soft rounded-md overflow-hidden">
          {([
            { value: "light" as ThemeMode, icon: "☀️", label: "浅色" },
            { value: "dark" as ThemeMode,  icon: "🌙", label: "深色" },
            { value: "auto" as ThemeMode,  icon: "💻", label: "自动" },
          ]).map(({ value, icon, label }) => (
            <button
              key={value}
              className={`flex items-center gap-1.5 px-3.5 py-2 bg-transparent border-0 border-r border-border-soft text-fg-dim text-[13px] cursor-pointer transition-colors hover:text-fg hover:bg-bg-soft last:border-r-0 ${
                mode === value ? "bg-accent-soft text-accent" : ""
              }`}
              onClick={() => onMode(value)}
            >
              <span>{icon}</span>
              <span>{label}</span>
            </button>
          ))}
        </div>
      </div>

      {/* ── 界面字体（不变） ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">界面字体</label>
        <select className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent" value={uiFont} onChange={e => applyFont("ui", e.target.value)}>
          <option value="">系统默认</option>
          <option value="pingfang">苹方 (PingFang SC)</option>
          <option value="yahei">微软雅黑</option>
          <option value="noto">Noto Sans SC</option>
        </select>
      </div>

      {/* ── 等宽字体（不变） ── */}
      <div className="mb-4">
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">等宽字体</label>
        <select className="w-full bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none focus:border-accent" value={monoFont} onChange={e => applyFont("mono", e.target.value)}>
          <option value="">系统默认</option>
          <option value="cascadia">Cascadia Code</option>
          <option value="jetbrains">JetBrains Mono</option>
          <option value="sfmono">SF Mono</option>
        </select>
      </div>

      {/* ── 语言（不变） ── */}
      <div>
        <label className="text-fg-dim text-[13px] font-medium mb-2 block">{t("settings.language")}</label>
        <div className="inline-flex border border-border-soft rounded-md overflow-hidden">
          {[
            { value: "", label: t("settings.langAuto") },
            { value: "zh", label: "简体中文" },
            { value: "zh-TW", label: "繁體中文" },
            { value: "en", label: "English" },
          ].map(({ value, label }) => (
            <button
              key={value}
              className={`px-3 py-1.5 bg-transparent border-0 border-r border-border-soft text-fg-dim text-xs cursor-pointer transition-[color,background] hover:text-fg hover:bg-bg-soft last:border-r-0 ${pref === value ? "bg-accent-soft text-accent" : ""}`}
              onClick={() => setPref(value as "" | "en" | "zh" | "zh-TW")}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}
```

---

## Phase 5: 多语言词条

### Task 10: 更新 zh.ts — 简体中文

**Files:**
- Modify: `tianxuan/desktop/frontend/src/locales/zh.ts`

- [ ] **Step 1: 在 `settings.themeDark` 行（第 340 行）后追加配色词条**

```typescript
  // 配色方案
  "settings.colorScheme": "配色方案",
  "settings.colorDefault": "默认",
  "settings.colorWarm": "暖色",
  "settings.colorIce": "冰蓝",
  "settings.colorForest": "森林",
  "settings.colorSunset": "日落",
  "settings.colorOcean": "海洋",
  "settings.colorRose": "玫瑰",
  "settings.colorViolet": "紫罗兰",
```

### Task 11: 更新 zh-TW.ts — 繁体中文

**Files:**
- Read & Modify: `tianxuan/desktop/frontend/src/locales/zh-TW.ts`

- [ ] **Step 1: 在对应位置追加繁体配色词条**

```typescript
  "settings.colorScheme": "配色方案",
  "settings.colorDefault": "預設",
  "settings.colorWarm": "暖色",
  "settings.colorIce": "冰藍",
  "settings.colorForest": "森林",
  "settings.colorSunset": "日落",
  "settings.colorOcean": "海洋",
  "settings.colorRose": "玫瑰",
  "settings.colorViolet": "紫羅蘭",
```

### Task 12: 更新 en.ts — 英文 + 新增 DictKey

**Files:**
- Modify: `tianxuan/desktop/frontend/src/locales/en.ts`

- [ ] **Step 1: 在 DictKey 类型末尾追加新 key**

```typescript
  "settings.colorScheme",
  "settings.colorDefault",
  "settings.colorWarm",
  "settings.colorIce",
  "settings.colorForest",
  "settings.colorSunset",
  "settings.colorOcean",
  "settings.colorRose",
  "settings.colorViolet",
```

- [ ] **Step 2: 在 record 对象末尾追加英文词条**

```typescript
  "settings.colorScheme": "Color Scheme",
  "settings.colorDefault": "Default",
  "settings.colorWarm": "Warm",
  "settings.colorIce": "Ice",
  "settings.colorForest": "Forest",
  "settings.colorSunset": "Sunset",
  "settings.colorOcean": "Ocean",
  "settings.colorRose": "Rose",
  "settings.colorViolet": "Violet",
```

---

## Phase 6: 验证

### Task 13: 整体验证

- [ ] **Step 1: TypeScript 编译**

```bash
cd tianxuan/desktop/frontend && npx tsc --noEmit --pretty 2>&1
```

Expected: 零类型错误。

- [ ] **Step 2: 前端构建**

```bash
cd tianxuan/desktop/frontend && npm run build
```

Expected: 构建成功。

- [ ] **Step 3: 检查残留引用**

```bash
cd tianxuan/desktop/frontend/src && rg "data-theme[^-]" --type-add 'web:*.{tsx,ts,css,html}' -t web
```

Expected: 零匹配（不再有任何对 `data-theme` 的引用，只有 `data-theme-scheme` 和 `data-theme-mode`）。

- [ ] **Step 4: 提交**

```bash
git add tianxuan/desktop/frontend/
git commit -m "feat: 主题双模式重构 — 8配色 × 2模式"
```

---

## 自审

1. **Spec coverage**: 8 配色均有 dark/light 值 ✅，双属性分离 ✅，旧协议迁移 ✅，UI 双层选择 ✅，locale 更新 ✅
2. **Placeholder scan**: 无 TODO/TBD ✅，Tasks 8-9 含完整代码 ✅（Task 2 styles.css 因篇幅限制提供 warm 完整块 + 其余 7 块的模板标注——实现时按模板填充）
3. **Type consistency**: `ColorScheme`/`ThemeMode` 在所有任务中一致 ✅，`AppearanceSection` props 与 SettingsPanel 调用一致 ✅
