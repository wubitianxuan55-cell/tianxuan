// browserHistory.ts — 浏览器面板的导航历史栈（纯函数，可测试）。
// 语义与浏览器前进/后退一致：entries 是访问过的 URL 序列，index 指向当前页。

export interface BrowserHistory {
  entries: string[];
  index: number;
}

export function createBrowserHistory(): BrowserHistory {
  return { entries: [], index: -1 };
}

export function canGoBack(h: BrowserHistory): boolean {
  return h.index > 0;
}

export function canGoForward(h: BrowserHistory): boolean {
  return h.index >= 0 && h.index < h.entries.length - 1;
}

// goBack / goForward 返回新对象，不原地修改。
export function goBack(h: BrowserHistory): BrowserHistory {
  if (!canGoBack(h)) return h;
  return { entries: h.entries, index: h.index - 1 };
}

export function goForward(h: BrowserHistory): BrowserHistory {
  if (!canGoForward(h)) return h;
  return { entries: h.entries, index: h.index + 1 };
}

// push 访问新 URL：若与当前页相同则忽略；否则裁剪掉前进分支再追加
// （浏览器行为：新导航清空 forward 历史）。
export function pushHistory(h: BrowserHistory, url: string): BrowserHistory {
  const normalized = url.trim();
  if (normalized === "") return h;
  if (h.index >= 0 && h.entries[h.index] === normalized) return h;
  const entries = h.entries.slice(0, h.index + 1);
  entries.push(normalized);
  return { entries, index: entries.length - 1 };
}

// normalizeUrl 把用户输入规范化为可加载的 http(s) URL：
// 裸域名/主机补 https://，已经是完整 URL 的不动。
export function normalizeUrl(input: string): string {
  const raw = input.trim();
  if (raw === "") return "";
  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(raw)) return raw;
  if (raw.startsWith("//")) return "https:" + raw;
  return "https://" + raw;
}
