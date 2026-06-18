// 布局常量 + clamp 函数 + localStorage 宽度持久化
// 从 App.tsx 提取，纯函数模块

import { loadLayoutSize, saveLayoutSize } from "../lib/layoutPreferences";

export const SIDEBAR_COLLAPSED_KEY = "tianxuan.sidebar.collapsed";
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

export function clampWorkspacePanelWidth(
  width: number,
  sidebarWidth = SIDEBAR_DEFAULT_WIDTH,
  viewportWidth = 1440,
): number {
  const maxByRatio = Math.floor(viewportWidth * WORKSPACE_PANEL_MAX_RATIO);
  const maxByChat = Math.floor(viewportWidth - sidebarWidth - CHAT_MIN_WIDTH);
  const max = Math.max(
    WORKSPACE_PANEL_MIN_WIDTH,
    Math.min(WORKSPACE_PANEL_MAX_WIDTH, maxByRatio, maxByChat),
  );
  return Math.min(max, Math.max(WORKSPACE_PANEL_MIN_WIDTH, Math.round(width)));
}

export function clampWorkspaceFileTreePanelWidth(
  width: number,
  sidebarWidth = SIDEBAR_DEFAULT_WIDTH,
  viewportWidth = 1440,
): number {
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
  try {
    return window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

export function saveSidebarCollapsed(collapsed: boolean): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, collapsed ? "1" : "0");
  } catch {
    /* ignore storage failures */
  }
}

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
  return loadLayoutSize(
    "workspaceFileTreePanelWidth",
    WORKSPACE_FILE_TREE_PANEL_DEFAULT_WIDTH,
    clampWorkspaceFileTreePanelWidth,
  );
}

export function saveWorkspaceFileTreePanelWidth(width: number): void {
  saveLayoutSize("workspaceFileTreePanelWidth", width);
}
