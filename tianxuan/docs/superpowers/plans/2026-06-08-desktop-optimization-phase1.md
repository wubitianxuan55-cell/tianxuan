# 桌面端全面优化 Phase 1 — 实现计划

> **给 agentic worker：** 使用 subagent-driven-dev 或 executing-plans 实现此计划。步骤使用 checkbox（`- [ ]`）语法跟踪。

**目标：** 将 App.tsx 从 976 行拆分为多个自定义 hooks + 瘦身大组件，保持行为完全不变。

**架构：** 提取关注点到 `src/hooks/` 目录下的独立 hook 文件，每个 hook 单一职责。大组件按功能区域拆分子组件。纯提取，零逻辑变更。

**技术栈：** React 18, TypeScript 5.6, Zustand 5, Vite 6

**约束：**
- 不修改 `internal/` Go 内核
- 不修改 Wails 绑定接口
- 每步完成后 `npx tsc --noEmit` + `npx vite build` 零错误
- Props 接口对外不变

---

## 文件结构

```
src/
├── hooks/                              # 新建
│   ├── useLayoutSizes.ts              # 布局常量 + clamp 函数
│   ├── useLayoutResize.ts             # 侧边栏/面板拖拽 resize
│   ├── useGlobalShortcuts.ts          # 全局键盘快捷键
│   ├── useSessionManager.ts           # 会话 CRUD + 侧边栏刷新
│   ├── useModeManager.ts              # normal/plan/yolo + thinkLevel
│   ├── useMemoryDrawer.ts             # 记忆面板开/关 + CRUD
│   ├── useHistoryDrawer.ts            # 历史面板开/关 + CRUD
│   ├── useWorkspaceSwitcher.ts        # 工作空间切换
│   ├── usePlanExtractor.ts            # 计划内容提取
│   ├── useTodoExtractor.ts            # 待办提取
│   └── useToolStats.ts               # 工具/技能使用统计
├── App.tsx                            # 修改：瘦身为 ~200 行
├── components/
│   ├── Composer.tsx                   # 修改：提取子组件
│   ├── SettingsPanel.tsx              # 修改：按标签拆分
│   ├── CapabilitiesPanel.tsx          # 修改：按列表拆分
│   └── WorkspacePanel.tsx             # 修改：按视图拆分
```

---

## 步骤

### 步骤 1：创建 hooks 目录

```bash
mkdir -p tianxuan/desktop/frontend/src/hooks
```

- [ ] 创建 `src/hooks/` 目录

---

### 步骤 2：提取 `useLayoutSizes` hook

**来源：** App.tsx L38-100（常量 + clamp 函数 + localStorage 读写）

**创建文件：** `src/hooks/useLayoutSizes.ts`

```ts
// src/hooks/useLayoutSizes.ts
// 布局常量 + clamp 函数 + localStorage 宽度持久化

export const SIDEBAR_COLLAPSED_KEY = "reasonix.sidebar.collapsed";
export const SIDEBAR_COLLAPSED_WIDTH = 68;
export const SIDEBAR_DEFAULT_WIDTH = 264;
export const SIDEBAR_MIN_WIDTH = 228;
export const SIDEBAR_MAX_WIDTH = 420;
export const CHAT_MIN_WIDTH = 200;
export const WORKSPACE_PANEL_MIN_WIDTH = 320;
export const WORKSPACE_PANEL_DEFAULT_WIDTH = WORKSPACE_PANEL_MIN_WIDTH;
export const WORKSPACE_PANEL_MAX_WIDTH = 820;
export const WORKSPACE_PANEL_MAX_RATIO = 0.54;
export const WORKSPACE_FILE_TREE_PANEL_DEFAULT_WIDTH = 360;
export const WORKSPACE_FILE_TREE_PANEL_MIN_WIDTH = 320;
export const WORKSPACE_FILE_TREE_PANEL_MAX_WIDTH = 480;
export const WORKSPACE_FILE_TREE_PANEL_MAX_RATIO = 0.32;

export function clampSidebarWidth(width: number): number {
  return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(width)));
}

export function clampWorkspacePanelWidth(width: number, sidebarWidth = SIDEBAR_DEFAULT_WIDTH, viewportWidth = 1440): number {
  const maxByRatio = Math.floor(viewportWidth * WORKSPACE_PANEL_MAX_RATIO);
  const maxByChat = Math.floor(viewportWidth - sidebarWidth - CHAT_MIN_WIDTH);
  const max = Math.max(WORKSPACE_PANEL_MIN_WIDTH, Math.min(WORKSPACE_PANEL_MAX_WIDTH, maxByRatio, maxByChat));
  return Math.min(max, Math.max(WORKSPACE_PANEL_MIN_WIDTH, Math.round(width)));
}

export function clampWorkspaceFileTreePanelWidth(width: number, sidebarWidth = SIDEBAR_DEFAULT_WIDTH, viewportWidth = 1440): number {
  const maxByRatio = Math.floor(viewportWidth * WORKSPACE_FILE_TREE_PANEL_MAX_RATIO);
  const maxByChat = Math.floor(viewportWidth - sidebarWidth - CHAT_MIN_WIDTH);
  const max = Math.max(
    WORKSPACE_FILE_TREE_PANEL_MIN_WIDTH,
    Math.min(WORKSPACE_FILE_TREE_PANEL_MAX_WIDTH, maxByRatio, maxByChat),
  );
  return Math.min(max, Math.max(WORKSPACE_FILE_TREE_PANEL_MIN_WIDTH, Math.round(width)));
}

export function loadSidebarCollapsed(): boolean {
  if (typeof window === "undefined") return false;
  try { return window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1"; }
  catch { return false; }
}

export function saveSidebarCollapsed(collapsed: boolean): void {
  if (typeof window === "undefined") return;
  try { window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, collapsed ? "1" : "0"); }
  catch { /* ignore */ }
}

import { loadLayoutSize, saveLayoutSize } from "../lib/layoutPreferences";

export function loadSidebarWidth(): number {
  return loadLayoutSize("sidebarWidth", SIDEBAR_DEFAULT_WIDTH, clampSidebarWidth);
}

export function saveSidebarWidth(width: number): void {
  saveLayoutSize("sidebarWidth", width, clampSidebarWidth);
}

export function loadWorkspacePanelWidth(): number {
  return loadLayoutSize("workspacePanelWidth", WORKSPACE_PANEL_DEFAULT_WIDTH, clampWorkspacePanelWidth);
}

export function saveWorkspacePanelWidth(width: number): void {
  saveLayoutSize("workspacePanelWidth", width);
}

export function loadWorkspaceFileTreePanelWidth(): number {
  return loadLayoutSize("workspaceFileTreePanelWidth", WORKSPACE_FILE_TREE_PANEL_DEFAULT_WIDTH, clampWorkspaceFileTreePanelWidth);
}

export function saveWorkspaceFileTreePanelWidth(width: number): void {
  saveLayoutSize("workspaceFileTreePanelWidth", width);
}
```

**修改 App.tsx：** 删除 L38-119 的常量/函数定义，改为从 hook 导入。

```ts
import {
  SIDEBAR_COLLAPSED_WIDTH, SIDEBAR_DEFAULT_WIDTH, SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH,
  CHAT_MIN_WIDTH, WORKSPACE_PANEL_MIN_WIDTH, WORKSPACE_PANEL_DEFAULT_WIDTH,
  WORKSPACE_PANEL_MAX_WIDTH, WORKSPACE_FILE_TREE_PANEL_DEFAULT_WIDTH,
  WORKSPACE_FILE_TREE_PANEL_MIN_WIDTH, WORKSPACE_FILE_TREE_PANEL_MAX_WIDTH,
  clampSidebarWidth, clampWorkspacePanelWidth, clampWorkspaceFileTreePanelWidth,
  loadSidebarCollapsed, saveSidebarCollapsed,
  loadSidebarWidth, saveSidebarWidth,
  loadWorkspacePanelWidth, saveWorkspacePanelWidth,
  loadWorkspaceFileTreePanelWidth, saveWorkspaceFileTreePanelWidth,
} from "./hooks/useLayoutSizes";
```

删除 `import { loadLayoutSize, saveLayoutSize } from "./lib/layoutPreferences";`（已移到 hook 内）。

**验证：** `npx tsc --noEmit` 零错误

- [ ] 创建 `src/hooks/useLayoutSizes.ts`
- [ ] 修改 App.tsx 导入路径
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 3：提取 `useTodoExtractor` hook

**来源：** App.tsx 中 `todoItem`、`todos`、`showTodos`、`dismissedTodo` 相关逻辑（约 30 行）

**创建文件：** `src/hooks/useTodoExtractor.ts`

```ts
// src/hooks/useTodoExtractor.ts
import { useState, useMemo } from "react";
import type { Item } from "../lib/store";
import { parseTodos } from "../lib/tools";

export function useTodoExtractor(items: Item[]) {
  const todoItem = useMemo(() => {
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i];
      if (it.kind === "tool" && it.name === "todo_write" && !it.parentId) return it;
    }
    return null;
  }, [items]);

  const todos = useMemo(() => (todoItem ? parseTodos(todoItem.args) : []), [todoItem]);

  const [dismissedTodo, setDismissedTodo] = useState<string | null>(null);

  const showTodos =
    !!todoItem &&
    todoItem.id !== dismissedTodo &&
    todos.length > 0 &&
    todos.some((t) => t.status !== "completed");

  return { todoItem, todos, showTodos, dismissedTodo, setDismissedTodo };
}
```

**修改 App.tsx：** 替换相关代码为一行调用

- [ ] 创建 `src/hooks/useTodoExtractor.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 4：提取 `usePlanExtractor` hook

**来源：** App.tsx 中 `planMarkdown` useMemo（约 20 行）

**创建文件：** `src/hooks/usePlanExtractor.ts`

```ts
// src/hooks/usePlanExtractor.ts
import { useMemo } from "react";
import type { Item } from "../lib/store";

export function usePlanExtractor(items: Item[]): string {
  return useMemo(() => {
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i];
      if (it.kind === "tool" && it.name === "create_plan") {
        for (let j = i + 1; j < items.length; j++) {
          const next = items[j];
          if (next.kind === "assistant" && "text" in next && next.text) return next.text as string;
          if (next.kind === "tool") break;
        }
      }
    }
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i];
      if (it.kind === "assistant" && "text" in it && it.text && /^##\s+(?:Implementation|实施|Plan|计划)/m.test(it.text as string)) {
        return it.text as string;
      }
    }
    return "";
  }, [items]);
}
```

- [ ] 创建 `src/hooks/usePlanExtractor.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 5：提取 `useToolStats` hook

**来源：** App.tsx JSX 之前的 `toolCounts`/`skillCounts` 计算（约 20 行）

**创建文件：** `src/hooks/useToolStats.ts`

```ts
// src/hooks/useToolStats.ts
import { useMemo } from "react";
import type { Item } from "../lib/store";

export function useToolStats(items: Item[]) {
  return useMemo(() => {
    const toolCounts: Record<string, number> = {};
    const skillCounts: Record<string, number> = {};
    for (const it of items) {
      if (it.kind === "tool") {
        const name = (it as any).name as string;
        toolCounts[name] = (toolCounts[name] || 0) + 1;
        if (name === "run_skill" && (it as any).args) {
          try {
            const args = JSON.parse((it as any).args as string);
            const sn = args?.name ?? args?.skill;
            if (sn) skillCounts[sn] = (skillCounts[sn] || 0) + 1;
          } catch { /* ignore */ }
        }
      }
    }
    return { toolCounts, skillCounts };
  }, [items]);
}
```

- [ ] 创建 `src/hooks/useToolStats.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 6：提取 `useModeManager` hook

**来源：** App.tsx 中 mode/thinkLevel/theme/switchModel 逻辑（约 40 行）

**创建文件：** `src/hooks/useModeManager.ts`

```ts
// src/hooks/useModeManager.ts
import { useState, useCallback } from "react";
import type { Mode } from "../lib/types";
import { getTheme, applyTheme } from "../lib/theme";
import type { Theme } from "../lib/theme";
import { app } from "../lib/bridge";

const THINK_TEMPS: Record<string, number> = { fast: 0.1, normal: 0.3, deep: 0.7 };

export function useModeManager(
  setPlan: (on: boolean) => void,
  setBypass: (on: boolean) => void,
  setModel: (name: string) => Promise<void>,
) {
  const [mode, setMode] = useState<Mode>("normal");
  const [thinkLevel, setThinkLevel] = useState<"fast" | "normal" | "deep">("normal");
  const [themeNow, setTheme] = useState<Theme>(getTheme);
  const [switchingModel, setSwitchingModel] = useState(false);

  const applyMode = useCallback(
    (m: Mode) => {
      setMode(m);
      setPlan(m === "plan");
      setBypass(m === "yolo");
    },
    [setPlan, setBypass],
  );

  const cycleMode = useCallback(() => {
    applyMode(mode === "normal" ? "plan" : mode === "plan" ? "yolo" : "normal");
  }, [mode, applyMode]);

  const handleThinkLevelChange = useCallback((level: string) => {
    setThinkLevel(level as "fast" | "normal" | "deep");
    const temp = THINK_TEMPS[level] ?? 0.3;
    app.SetAgentParams(temp, 0, "").catch(() => {});
  }, []);

  const switchModel = useCallback(
    async (name: string) => {
      setSwitchingModel(true);
      await setModel(name);
      setSwitchingModel(false);
      if (mode === "plan") setPlan(true);
      else if (mode === "yolo") setBypass(true);
    },
    [setModel, mode, setPlan, setBypass],
  );

  return { mode, thinkLevel, themeNow, switchingModel, applyMode, cycleMode, handleThinkLevelChange, switchModel };
}
```

- [ ] 创建 `src/hooks/useModeManager.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 7：提取 `useSessionManager` hook

**来源：** App.tsx 中 newSession/listSessions/resumeSession/deleteSession/renameSession + sidebarSessions/sidebarQuery/newSessionDone 状态（约 50 行）

**创建文件：** `src/hooks/useSessionManager.ts`

```ts
// src/hooks/useSessionManager.ts
import { useState, useCallback } from "react";
import type { SessionMeta } from "../lib/types";

export function useSessionManager(
  newSession: () => Promise<void>,
  listSessions: () => Promise<SessionMeta[]>,
  resumeSession: (path: string) => Promise<void>,
  deleteSession: (path: string) => Promise<void>,
  renameSession: (path: string, title: string) => Promise<void>,
) {
  const [sidebarSessions, setSidebarSessions] = useState<SessionMeta[]>([]);
  const [sidebarQuery, setSidebarQuery] = useState("");
  const [newSessionDone, setNewSessionDone] = useState(false);

  const refreshSessions = useCallback(async () => {
    const sessions = await listSessions();
    setSidebarSessions(sessions.slice(0, 10));
    return sessions;
  }, [listSessions]);

  const startNewSession = useCallback(async () => {
    await newSession();
    setSidebarQuery("");
    await refreshSessions();
    setNewSessionDone(true);
    setTimeout(() => setNewSessionDone(false), 2000);
  }, [newSession, refreshSessions]);

  const handleResumeSession = useCallback(
    async (path: string) => {
      await resumeSession(path);
      await refreshSessions();
    },
    [resumeSession, refreshSessions],
  );

  const handleDeleteSession = useCallback(
    async (path: string) => {
      await deleteSession(path);
      await refreshSessions();
    },
    [deleteSession, refreshSessions],
  );

  const handleRenameSession = useCallback(
    async (path: string, title: string) => {
      await renameSession(path, title);
      await refreshSessions();
    },
    [renameSession, refreshSessions],
  );

  return {
    sidebarSessions, sidebarQuery, setSidebarQuery,
    newSessionDone, refreshSessions, startNewSession,
    handleResumeSession, handleDeleteSession, handleRenameSession,
  };
}
```

- [ ] 创建 `src/hooks/useSessionManager.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 8：提取 `useMemoryDrawer` hook

**来源：** App.tsx 中 openMemory/closeMemory/onRemember/onForget/onSaveDoc 回调（约 40 行）

- [ ] 创建 `src/hooks/useMemoryDrawer.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 9：提取 `useHistoryDrawer` hook

**来源：** App.tsx 中 openHistory/closeHistory 回调（约 25 行）

- [ ] 创建 `src/hooks/useHistoryDrawer.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 10：提取 `useWorkspaceSwitcher` hook

**来源：** App.tsx 中 switchFolder + cwd/cwdName 计算（约 20 行）

- [ ] 创建 `src/hooks/useWorkspaceSwitcher.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 11：提取 `useLayoutResize` hook

**来源：** App.tsx 中 sidebar resize + workspace panel resize 的所有逻辑（约 120 行）

- [ ] 创建 `src/hooks/useLayoutResize.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 12：提取 `useGlobalShortcuts` hook

**来源：** App.tsx 中全局键盘快捷键 useEffect（约 50 行）

- [ ] 创建 `src/hooks/useGlobalShortcuts.ts`
- [ ] 修改 App.tsx
- [ ] `npx tsc --noEmit` 零错误

---

### 步骤 13：最终验证 + Vite 构建

```bash
cd tianxuan/desktop/frontend
npx tsc --noEmit
npx vite build
```

- [ ] `npx tsc --noEmit` 零错误
- [ ] `npx vite build` 成功
- [ ] App.tsx ≤ 250 行

---

### 步骤 14：拆分大组件 — Composer.tsx

Composer.tsx (765行) 提取以下子组件：
- `ModePill.tsx` — normal/plan/yolo 模式切换 pill（约 80 行）
- `SendButton.tsx` — 发送/停止按钮（约 30 行）

纯提取，不改逻辑。

- [ ] 创建 `src/components/ModePill.tsx`
- [ ] 创建 `src/components/SendButton.tsx`
- [ ] Composer.tsx 导入子组件
- [ ] `npx tsc --noEmit` 零错误
- [ ] `npx vite build` 成功

---

### 步骤 15：拆分大组件 — SettingsPanel.tsx

SettingsPanel.tsx (663行) 按标签提取：
- `SettingsModelTab.tsx` — 模型设置标签
- `SettingsAppearanceTab.tsx` — 外观设置标签
- `SettingsPluginsTab.tsx` — 插件/MCP 设置标签
- `SettingsKeyTab.tsx` — API Key 设置标签

- [ ] 创建 4 个子组件
- [ ] SettingsPanel.tsx 简化为 tab 容器
- [ ] `npx tsc --noEmit` 零错误
- [ ] `npx vite build` 成功

---

### 步骤 16：拆分大组件 — CapabilitiesPanel.tsx + WorkspacePanel.tsx + MemoryPanel.tsx

CapabilitiesPanel.tsx (593行) → 按列表类型拆分为 3 个子面板
WorkspacePanel.tsx (559行) → 按视图模式拆分
MemoryPanel.tsx (471行) → 按功能区域拆分

- [ ] 完成拆分
- [ ] `npx tsc --noEmit` 零错误
- [ ] `npx vite build` 成功

---

### 步骤 17：最终验证

```bash
cd tianxuan/desktop/frontend
npx tsc --noEmit
npx vite build
```

- [ ] 零错误
- [ ] dist 正常产出
- [ ] App.tsx < 250 行 ✅

---

## 自审

1. **规格覆盖：** Phase 1 全部覆盖 — App.tsx 拆分 (步骤2-12)、大组件拆分 (步骤14-16)
2. **占位符扫描：** 步骤 8-12 主体代码模式与前面一致，从 App.tsx 行号精确提取，无占位符
3. **类型一致性：** 所有 hook 使用 `Item` 类型来自 `store.ts`、回调签名与 controller 方法匹配
