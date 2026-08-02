// BrowserPanel — 右侧面板"浏览器" tab：像 Codex 一样在应用内查看网页。
// 页面模式用 iframe 渲染（WebView2 完整渲染）；网站拒绝 iframe 嵌入
// （X-Frame-Options / CSP）或用户想读纯文本时，切换到文本模式——复用内核
// web_fetch（SSRF 防护 + 去标签），保证任何站点都能"查看阅览"。
import { useCallback, useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Eye,
  FileText,
  Globe,
  Loader2,
  RefreshCw,
  Search,
} from "lucide-react";
import { app } from "../lib/bridge";
import {
  canGoBack,
  canGoForward,
  createBrowserHistory,
  goBack,
  goForward,
  normalizeUrl,
  pushHistory,
  type BrowserHistory,
} from "../lib/browserHistory";
import { useT } from "../lib/i18n";

type BrowserMode = "page" | "text";

export function BrowserPanel() {
  const t = useT();
  const [input, setInput] = useState("");
  const [history, setHistory] = useState<BrowserHistory>(createBrowserHistory());
  const [mode, setMode] = useState<BrowserMode>("page");
  const [textContent, setTextContent] = useState("");
  const [textLoading, setTextLoading] = useState(false);
  const [textError, setTextError] = useState("");
  const [iframeLoading, setIframeLoading] = useState(false);
  const [refreshTick, setRefreshTick] = useState(0);

  const current = history.index >= 0 ? history.entries[history.index] : "";

  const navigate = useCallback((raw: string) => {
    const url = normalizeUrl(raw);
    if (!url) return;
    setHistory((h) => pushHistory(h, url));
    setInput(url);
    setTextError("");
    setIframeLoading(true);
  }, []);

  const stepBack = useCallback(() => setHistory((h) => {
    const next = goBack(h);
    if (next !== h) setIframeLoading(true);
    return next;
  }), []);

  const stepForward = useCallback(() => setHistory((h) => {
    const next = goForward(h);
    if (next !== h) setIframeLoading(true);
    return next;
  }), []);

  const openText = useCallback(async (url: string) => {
    if (!url) return;
    setTextLoading(true);
    setTextError("");
    try {
      const out = await app.BrowserFetchText(url);
      setTextContent(out);
    } catch (e) {
      setTextError(String(e));
      setTextContent("");
    } finally {
      setTextLoading(false);
    }
  }, []);

  const switchMode = useCallback(
    (next: BrowserMode) => {
      setMode(next);
      if (next === "text" && current && !textContent) void openText(current);
    },
    [current, textContent, openText],
  );

  const onRefresh = useCallback(() => {
    if (!current) return;
    if (mode === "text") {
      void openText(current);
    } else {
      setIframeLoading(true);
      setRefreshTick((n) => n + 1);
    }
  }, [current, mode, openText]);

  const empty = current === "";

  return (
    <div className="flex flex-col h-full min-h-0">
      {/* ── 导航栏 ── */}
      <div className="flex items-center gap-1.5 px-2.5 py-2 border-b border-border-soft shrink-0">
        <button
          className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-colors duration-150 hover:bg-bg-soft hover:text-fg disabled:opacity-35 disabled:cursor-default"
          onClick={stepBack}
          disabled={!canGoBack(history)}
          title={t("browser.back")}
        >
          <ArrowLeft size={14} />
        </button>
        <button
          className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-colors duration-150 hover:bg-bg-soft hover:text-fg disabled:opacity-35 disabled:cursor-default"
          onClick={stepForward}
          disabled={!canGoForward(history)}
          title={t("browser.forward")}
        >
          <ArrowRight size={14} />
        </button>
        <form
          className="flex-1 min-w-0 flex items-center gap-1.5"
          onSubmit={(e) => { e.preventDefault(); navigate(input); }}
        >
          <div className="flex items-center gap-1.5 flex-1 min-w-0 border border-border-soft rounded-md bg-bg-soft px-2 py-1 focus-within:border-accent/40 transition-colors">
            <Globe size={12} className="text-fg-faint shrink-0" />
            <input
              className="flex-1 min-w-0 border-0 bg-transparent text-fg text-xs outline-none placeholder:text-fg-faint font-mono"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder={t("browser.placeholder")}
              spellCheck={false}
            />
          </div>
          <button
            type="submit"
            className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-accent text-accent-fg cursor-pointer transition-all duration-150 hover:brightness-110 active:scale-95 disabled:opacity-40 disabled:cursor-default"
            disabled={!input.trim()}
            title={t("browser.go")}
          >
            <Search size={13} />
          </button>
        </form>
        <button
          className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-colors duration-150 hover:bg-bg-soft hover:text-fg disabled:opacity-35 disabled:cursor-default"
          onClick={onRefresh}
          disabled={empty}
          title={t("browser.refresh")}
        >
          <RefreshCw size={13} />
        </button>
        <div className="flex items-center border border-border-soft rounded-md overflow-hidden shrink-0">
          <button
            className={`inline-flex items-center gap-1 px-2 py-1 text-[10px] border-0 cursor-pointer transition-colors duration-150 ${mode === "page" ? "bg-accent/12 text-accent" : "bg-transparent text-fg-faint hover:text-fg"}`}
            onClick={() => switchMode("page")}
            title={t("browser.pageMode")}
          >
            <Eye size={11} />
            <span className="hidden lg:inline">{t("browser.pageMode")}</span>
          </button>
          <button
            className={`inline-flex items-center gap-1 px-2 py-1 text-[10px] border-0 cursor-pointer transition-colors duration-150 ${mode === "text" ? "bg-accent/12 text-accent" : "bg-transparent text-fg-faint hover:text-fg"}`}
            onClick={() => switchMode("text")}
            title={t("browser.textMode")}
          >
            <FileText size={11} />
            <span className="hidden lg:inline">{t("browser.textMode")}</span>
          </button>
        </div>
      </div>

      {/* ── 内容区 ── */}
      <div className="flex-1 min-h-0 relative">
        {empty ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-fg-faint/50">
            <Globe size={28} />
            <div className="text-xs">{t("browser.emptyHint")}</div>
          </div>
        ) : mode === "page" ? (
          <iframe
            key={`${current}#${refreshTick}`}
            src={current}
            className="w-full h-full border-0 bg-white"
            sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-downloads"
            referrerPolicy="no-referrer"
            onLoad={() => setIframeLoading(false)}
            title={current}
          />
        ) : textLoading ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-fg-faint/60">
            <Loader2 size={22} className="animate-spin text-accent" />
            <div className="text-xs font-mono">{t("browser.loading")}</div>
          </div>
        ) : textError ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 p-6 text-center">
            <div className="text-xs text-err font-mono break-all">{textError}</div>
          </div>
        ) : (
          <pre className="h-full overflow-auto p-3 text-[11.5px] leading-relaxed text-fg-dim font-mono whitespace-pre-wrap break-words">
            {textContent}
          </pre>
        )}

        {mode === "page" && iframeLoading && (
          <div className="absolute top-1 left-1/2 -translate-x-1/2 z-10 flex items-center gap-1.5 px-2.5 py-1 rounded-full border border-border-soft bg-bg-elev text-fg-faint text-[10px] shadow-[var(--ds-shadow-dropdown)]">
            <Loader2 size={11} className="animate-spin text-accent" />
            <span className="font-mono">{t("browser.loading")}</span>
          </div>
        )}
      </div>
    </div>
  );
}
