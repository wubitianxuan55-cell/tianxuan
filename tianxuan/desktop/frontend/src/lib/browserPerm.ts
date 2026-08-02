// browserPerm.ts — 浏览器域名白名单（仿 Codex 的 allowed hosts）。
// 首次访问未授权域名时由 UI 弹确认，确认后写入 localStorage；再次访问免确认。
// storage 以参数注入便于在 node 测试环境中用内存 mock 验证。
export const BROWSER_ALLOW_KEY = "tianxuan.browser.allowedHosts";

export type BrowserStorage = Pick<Storage, "getItem" | "setItem">;

// hostOf 返回 http(s) URL 的小写 hostname；非 http(s) 或不可解析返回 null。
export function hostOf(url: string): string | null {
  try {
    const u = new URL(url);
    if (u.protocol !== "http:" && u.protocol !== "https:") return null;
    return u.hostname.toLowerCase();
  } catch {
    return null;
  }
}

export function getAllowedHosts(storage: BrowserStorage): string[] {
  try {
    const raw = storage.getItem(BROWSER_ALLOW_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((h): h is string => typeof h === "string" && h.length > 0);
  } catch {
    return [];
  }
}

export function isAllowedHost(host: string, storage: BrowserStorage): boolean {
  return getAllowedHosts(storage).includes(host.toLowerCase());
}

export function allowHost(host: string, storage: BrowserStorage): string[] {
  const h = host.toLowerCase();
  const next = getAllowedHosts(storage);
  if (!next.includes(h)) next.push(h);
  storage.setItem(BROWSER_ALLOW_KEY, JSON.stringify(next));
  return next;
}
