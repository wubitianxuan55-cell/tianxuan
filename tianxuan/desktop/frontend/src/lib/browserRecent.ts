// browserRecent.ts — 跨会话"最近访问"列表（新标签页起始页用）。
// 按域名去重（同域名只留最新一条并移到最前），上限默认 8 条，localStorage 持久化。
import { hostOf, type BrowserStorage } from "./browserPerm";

export const BROWSER_RECENT_KEY = "tianxuan.browser.recent";
export const RECENT_DEFAULT_MAX = 8;

export interface RecentVisit {
  host: string;
  url: string;
  at: number;
}

function isRecentVisit(v: unknown): v is RecentVisit {
  if (typeof v !== "object" || v === null) return false;
  const o = v as Record<string, unknown>;
  return typeof o.host === "string" && typeof o.url === "string" && typeof o.at === "number";
}

export function getRecent(storage: BrowserStorage, max = RECENT_DEFAULT_MAX): RecentVisit[] {
  try {
    const raw = storage.getItem(BROWSER_RECENT_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isRecentVisit).slice(0, max);
  } catch {
    return [];
  }
}

export function recordVisit(storage: BrowserStorage, url: string, at = Date.now(), max = RECENT_DEFAULT_MAX): RecentVisit[] {
  const host = hostOf(url);
  if (!host) return getRecent(storage, max);
  const entry: RecentVisit = { host, url, at };
  const next = [entry, ...getRecent(storage, max).filter((r) => r.host !== host)].slice(0, max);
  storage.setItem(BROWSER_RECENT_KEY, JSON.stringify(next));
  return next;
}

export function clearRecent(storage: BrowserStorage): void {
  storage.setItem(BROWSER_RECENT_KEY, JSON.stringify([]));
}
