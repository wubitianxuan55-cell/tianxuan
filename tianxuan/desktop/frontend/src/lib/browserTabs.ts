// browserTabs.ts — 多标签浏览器的纯函数状态机（可测试）。
// 每个标签持有独立的导航历史栈与视图状态；App 层关闭最后一个标签时
// 应顺带关闭浏览器视图（由组件处理，此处只负责标签集合的增删切换）。
import { createBrowserHistory, type BrowserHistory } from "./browserHistory";

export type BrowserMode = "page" | "text";

export interface BrowserTab {
  id: string;
  history: BrowserHistory;
  mode: BrowserMode;
  input: string;
  textContent: string;
  textLoading: boolean;
  textError: string;
  iframeLoading: boolean;
  refreshTick: number;
  // pendingHost 是等待用户确认的域名（首次访问权限确认）；null 表示无待确认项。
  pendingHost: string | null;
}

function newTabId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `tab-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

export function createBrowserTab(id?: string): BrowserTab {
  return {
    id: id ?? newTabId(),
    history: createBrowserHistory(),
    mode: "page",
    input: "",
    textContent: "",
    textLoading: false,
    textError: "",
    iframeLoading: false,
    refreshTick: 0,
    pendingHost: null,
  };
}

// tabTitle 返回标签行显示名：当前页面的 host，空白页则返回空串。
export function tabTitle(tab: BrowserTab): string {
  const url = tab.history.index >= 0 ? tab.history.entries[tab.history.index] : "";
  if (url === "") return "";
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}

// hostInitial 返回域名首字母大写（标签行徽标用）；空或不可解析返回空串。
export function hostInitial(url: string): string {
  try {
    const host = new URL(url).hostname;
    const c = host.charAt(0);
    return c ? c.toUpperCase() : "";
  } catch {
    return "";
  }
}

export function addTab(tabs: BrowserTab[], _activeId: string): { tabs: BrowserTab[]; activeId: string } {
  const tab = createBrowserTab();
  return { tabs: [...tabs, tab], activeId: tab.id };
}

export function removeTab(
  tabs: BrowserTab[],
  activeId: string,
  id: string,
): { tabs: BrowserTab[]; activeId: string } {
  const idx = tabs.findIndex((t) => t.id === id);
  if (idx < 0) return { tabs, activeId };
  const next = tabs.filter((t) => t.id !== id);
  if (next.length === 0) return { tabs: next, activeId: "" };
  let active = activeId;
  if (active === id) {
    // 关闭的是当前标签：优先激活右邻居，没有则取左邻居。
    active = next[Math.min(idx, next.length - 1)].id;
  }
  return { tabs: next, activeId: active };
}

export function switchTab(tabs: BrowserTab[], id: string): string {
  return tabs.some((t) => t.id === id) ? id : "";
}

export function updateTab(tabs: BrowserTab[], id: string, patch: Partial<BrowserTab>): BrowserTab[] {
  return tabs.map((t) => (t.id === id ? { ...t, ...patch } : t));
}
