// browserBookmarks.ts — 跨会话"收藏"列表（书签）。
// 用户手动收藏的页面，按 URL 去重（同 URL 只留最新一条并移到最前），
// 上限默认 30 条，localStorage 持久化；storage 注入便于 node 测试。
import { hostOf, type BrowserStorage } from "./browserPerm";

export const BROWSER_BOOKMARK_KEY = "tianxuan.browser.bookmarks";
export const BOOKMARK_DEFAULT_MAX = 30;

export interface Bookmark {
  url: string;
  host: string;
  addedAt: number;
}

function isBookmark(v: unknown): v is Bookmark {
  if (typeof v !== "object" || v === null) return false;
  const o = v as Record<string, unknown>;
  return typeof o.url === "string" && typeof o.host === "string" && typeof o.addedAt === "number";
}

export function getBookmarks(storage: BrowserStorage, max = BOOKMARK_DEFAULT_MAX): Bookmark[] {
  try {
    const raw = storage.getItem(BROWSER_BOOKMARK_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isBookmark).slice(0, max);
  } catch {
    return [];
  }
}

export function addBookmark(storage: BrowserStorage, url: string, at = Date.now(), max = BOOKMARK_DEFAULT_MAX): Bookmark[] {
  const host = hostOf(url);
  if (!host) return getBookmarks(storage, max);
  const entry: Bookmark = { url, host, addedAt: at };
  const next = [entry, ...getBookmarks(storage, max).filter((b) => b.url !== url)].slice(0, max);
  storage.setItem(BROWSER_BOOKMARK_KEY, JSON.stringify(next));
  return next;
}

export function removeBookmark(storage: BrowserStorage, url: string): Bookmark[] {
  const next = getBookmarks(storage).filter((b) => b.url !== url);
  storage.setItem(BROWSER_BOOKMARK_KEY, JSON.stringify(next));
  return next;
}

export function isBookmarked(storage: BrowserStorage, url: string): boolean {
  return getBookmarks(storage).some((b) => b.url === url);
}
