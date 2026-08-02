// BrowserPanel — 仿 Codex Ctrl+Shift+B 的独立浏览器视图（替换聊天主区，非侧边面板）。
// 多标签，每标签独立历史栈；首次访问域名弹权限确认（localStorage 白名单，确认后免问）。
// 页面模式用 iframe 渲染（WebView2 完整渲染）；网站拒绝 iframe 嵌入或用户
// 想读纯文本时切换文本模式——复用内核 web_fetch（SSRF 防护 + 去标签）。
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Eye,
  ExternalLink,
  FileText,
  Globe,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Send,
  ShieldAlert,
  X,
} from "lucide-react";
import { app, openExternal } from "../lib/bridge";
import {
  canGoBack,
  canGoForward,
  goBack,
  goForward,
  normalizeUrl,
  pushHistory,
} from "../lib/browserHistory";
import {
  addTab,
  createBrowserTab,
  hostInitial,
  removeTab,
  switchTab,
  tabTitle,
  updateTab,
  type BrowserMode,
  type BrowserTab,
} from "../lib/browserTabs";
import {
  allowHost,
  hostOf,
  isAllowedHost,
  type BrowserStorage,
} from "../lib/browserPerm";
import { getRecent, recordVisit, type RecentVisit } from "../lib/browserRecent";
import { useT } from "../lib/i18n";

export function BrowserPanel({
  onClose,
  onSendText,
  visible,
}: {
  onClose: () => void;
  onSendText?: (text: string) => void;
  visible: boolean;
}) {
  const t = useT();
  const storage: BrowserStorage | null = useMemo(
    () => (typeof localStorage !== "undefined" ? localStorage : null),
    [],
  );
  const [tabs, setTabs] = useState<BrowserTab[]>(() => [createBrowserTab()]);
  const [activeId, setActiveId] = useState<string>("");
  const [selection, setSelection] = useState("");
  const [recent, setRecent] = useState<RecentVisit[]>(() => (storage ? getRecent(storage) : []));
  const addressRef = useRef<HTMLInputElement>(null);
  const currentId = activeId !== "" ? activeId : tabs[0]?.id ?? "";
  const active = tabs.find((tab) => tab.id === currentId) ?? tabs[0];
  const current = active?.history.index !== undefined && active.history.index >= 0
    ? active.history.entries[active.history.index]
    : "";

  // 导航/切换成功后记录"最近访问"（同域名去重并置顶）。
  useEffect(() => {
    if (current && storage) setRecent(recordVisit(storage, current));
  }, [current, storage]);

  // 提交一次真实导航（纯函数）：入栈历史、清空待确认域名与文本缓存、触发展示加载。
  const commitNavigate = useCallback((tab: BrowserTab, url: string): BrowserTab => ({
      ...tab,
      history: pushHistory(tab.history, url),
      input: url,
      pendingHost: null,
      textContent: "",
      textError: "",
      iframeLoading: true,
    }), []);

  // 导航入口：先过权限确认（首次访问域名），通过才真正加载。
  const navigate = useCallback((raw: string, targetId = currentId) => {
    const url = normalizeUrl(raw);
    if (!url) return;
    const host = hostOf(url);
    if (!host) return;
    setTabs((ts) => {
      const tab = ts.find((x) => x.id === targetId);
      if (!tab) return ts;
      return storage && isAllowedHost(host, storage)
        ? ts.map((x) => (x.id === targetId ? commitNavigate(x, url) : x))
        : updateTab(ts, targetId, { input: url, pendingHost: host });
    });
  }, [currentId, storage, commitNavigate]);

  const onAllow = useCallback((host: string, targetId: string) => {
    if (storage) allowHost(host, storage);
    setTabs((ts) =>
      ts.map((x) => (x.id === targetId ? commitNavigate(x, x.input) : x)),
    );
  }, [storage, commitNavigate]);

  const onDeny = useCallback((targetId: string) => {
    setTabs((ts) => updateTab(ts, targetId, { pendingHost: null }));
  }, []);

  const stepBack = useCallback(() => {
    if (!active) return;
    const next = goBack(active.history);
    if (next !== active.history) {
      setTabs((ts) => updateTab(ts, active.id, { history: next, iframeLoading: true }));
    }
  }, [active]);

  const stepForward = useCallback(() => {
    if (!active) return;
    const next = goForward(active.history);
    if (next !== active.history) {
      setTabs((ts) => updateTab(ts, active.id, { history: next, iframeLoading: true }));
    }
  }, [active]);

  const openText = useCallback(async (url: string, targetId: string) => {
    setTabs((ts) => updateTab(ts, targetId, { textLoading: true, textError: "" }));
    try {
      const out = await app.BrowserFetchText(url);
      setTabs((ts) => updateTab(ts, targetId, { textContent: out, textLoading: false }));
    } catch (e) {
      setTabs((ts) => updateTab(ts, targetId, { textError: String(e), textContent: "", textLoading: false }));
    }
  }, []);

  const switchMode = useCallback((next: BrowserMode) => {
    if (!active) return;
    setTabs((ts) => updateTab(ts, active.id, { mode: next }));
    if (next === "text" && current && !active.textContent) void openText(current, active.id);
  }, [active, current, openText]);

  const onRefresh = useCallback(() => {
    if (!active || !current) return;
    if (active.mode === "text") {
      void openText(current, active.id);
    } else {
      setTabs((ts) => updateTab(ts, active.id, {
        iframeLoading: true,
        refreshTick: (ts.find((x) => x.id === active.id)?.refreshTick ?? 0) + 1,
      }));
    }
  }, [active, current, openText]);

  const onNewTab = useCallback(() => {
    const r = addTab(tabs, activeId);
    setTabs(r.tabs);
    setActiveId(r.activeId);
    setSelection("");
  }, [tabs, activeId]);

  const onCloseTab = useCallback((id: string) => {
    const r = removeTab(tabs, activeId, id);
    if (r.tabs.length === 0) {
      onClose();
      return;
    }
    setTabs(r.tabs);
    setActiveId(r.activeId);
    setSelection("");
  }, [tabs, activeId, onClose]);

  // 浏览器视图可见时的快捷键：Ctrl+T 新标签 / Ctrl+W 关闭 / Ctrl+L 聚焦地址栏。
  useEffect(() => {
    if (!visible) return;
    const onKey = (e: globalThis.KeyboardEvent) => {
      const mod = e.ctrlKey || e.metaKey;
      if (!mod) return;
      const k = e.key.toLowerCase();
      if (k === "t") {
        e.preventDefault();
        onNewTab();
      } else if (k === "w") {
        e.preventDefault();
        onCloseTab(currentId);
      } else if (k === "l") {
        e.preventDefault();
        addressRef.current?.focus();
        addressRef.current?.select();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [visible, onNewTab, onCloseTab, currentId]);

  const switchTo = useCallback((id: string) => {
    const next = switchTab(tabs, id);
    if (next) {
      setActiveId(next);
      setSelection("");
    }
  }, [tabs]);

  const detectSelection = useCallback(() => {
    const sel = window.getSelection()?.toString().trim() ?? "";
    setSelection(sel);
  }, []);

  const sendSelection = useCallback(() => {
    if (!selection || !onSendText) return;
    onSendText(selection + (current ? `\n\n(来源: ${current})` : ""));
    setSelection("");
  }, [selection, onSendText, current]);

  const empty = !active || current === "";

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-bg">
      {/* 标签行：多标签 + 新建 */}
      <div className="flex items-end gap-1 px-2 pt-1.5 shrink-0 overflow-x-auto border-b border-border-soft bg-bg-soft/40">
        {tabs.map((tab) => {
          const isActive = tab.id === currentId;
          return (
            <div
              key={tab.id}
              className={`group flex items-center gap-1.5 min-w-0 max-w-[180px] px-2.5 py-1.5 rounded-t-md text-[11px] cursor-pointer select-none border border-b-0 transition-colors duration-150 ${
                isActive
                  ? "bg-bg border-border-soft text-fg"
                  : "bg-transparent border-transparent text-fg-faint hover:text-fg-dim"
              }`}
              onClick={() => switchTo(tab.id)}
            >
              {tabTitle(tab) ? (
                <span className={`inline-flex items-center justify-center w-4 h-4 rounded text-[9px] font-bold shrink-0 ${isActive ? "bg-accent/20 text-accent" : "bg-bg-soft text-fg-faint"}`}>
                  {hostInitial(tab.history.entries[tab.history.index] ?? "")}
                </span>
              ) : (
                <Globe size={11} className={isActive ? "text-accent shrink-0" : "shrink-0"} />
              )}
              <span className="truncate font-mono">
                {tabTitle(tab) || (t("browser.untitledTab") ?? "新标签页")}
              </span>
              <button
                className={`inline-flex items-center justify-center w-4 h-4 rounded border-0 cursor-pointer text-fg-faint hover:bg-bg-soft hover:text-fg ${
                  isActive ? "opacity-100" : "opacity-0 group-hover:opacity-100"
                }`}
                title={t("browser.closeTab") ?? "关闭标签页"}
                onClick={(e) => { e.stopPropagation(); onCloseTab(tab.id); }}
              >
                <X size={10} />
              </button>
            </div>
          );
        })}
        <button
          className="inline-flex items-center justify-center w-7 h-7 mb-0.5 border-0 rounded-md bg-transparent text-fg-faint cursor-pointer transition-colors duration-150 hover:bg-bg-soft hover:text-fg"
          title={t("browser.newTab") ?? "新标签页"}
          onClick={onNewTab}
        >
          <Plus size={14} />
        </button>
      </div>

      {/* 导航栏：关闭视图 + 前进后退 + 地址栏 + 刷新 + 双模式 */}
      <div className="flex items-center gap-1.5 px-2.5 py-2 border-b border-border-soft shrink-0">
        <button
          className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-colors duration-150 hover:bg-del-bg hover:text-err"
          onClick={onClose}
          title={t("browser.close") ?? "关闭浏览器"}
        >
          <X size={15} />
        </button>
        <button
          className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-colors duration-150 hover:bg-bg-soft hover:text-fg disabled:opacity-35 disabled:cursor-default"
          onClick={stepBack}
          disabled={!active || !canGoBack(active.history)}
          title={t("browser.back")}
        >
          <ArrowLeft size={14} />
        </button>
        <button
          className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-colors duration-150 hover:bg-bg-soft hover:text-fg disabled:opacity-35 disabled:cursor-default"
          onClick={stepForward}
          disabled={!active || !canGoForward(active.history)}
          title={t("browser.forward")}
        >
          <ArrowRight size={14} />
        </button>
        <form
          className="flex-1 min-w-0 flex items-center gap-1.5"
          onSubmit={(e) => { e.preventDefault(); navigate(active?.input ?? ""); }}
        >
          <div className="flex items-center gap-1.5 flex-1 min-w-0 border border-border-soft rounded-md bg-bg-soft px-2 py-1 focus-within:border-accent/40 transition-colors">
            <Globe size={12} className="text-fg-faint shrink-0" />
            <input
              ref={addressRef}
              className="flex-1 min-w-0 border-0 bg-transparent text-fg text-xs outline-none placeholder:text-fg-faint font-mono"
              value={active?.input ?? ""}
              onChange={(e) => {
                const v = e.target.value;
                setTabs((ts) => updateTab(ts, currentId, { input: v }));
              }}
              placeholder={t("browser.placeholder")}
              spellCheck={false}
            />
          </div>
          <button
            type="submit"
            className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-accent text-accent-fg cursor-pointer transition-all duration-150 hover:brightness-110 active:scale-95 disabled:opacity-40 disabled:cursor-default"
            disabled={!(active?.input.trim())}
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
        <button
          className="inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-dim cursor-pointer transition-colors duration-150 hover:bg-bg-soft hover:text-fg disabled:opacity-35 disabled:cursor-default"
          onClick={() => openExternal(current)}
          disabled={empty}
          title={t("browser.openExternal") ?? "在外部浏览器打开"}
        >
          <ExternalLink size={13} />
        </button>
        <div className="flex items-center border border-border-soft rounded-md overflow-hidden shrink-0">
          <button
            className={`inline-flex items-center gap-1 px-2 py-1 text-[10px] border-0 cursor-pointer transition-colors duration-150 ${active?.mode === "page" ? "bg-accent/12 text-accent" : "bg-transparent text-fg-faint hover:text-fg"}`}
            onClick={() => switchMode("page")}
            title={t("browser.pageMode")}
          >
            <Eye size={11} />
            <span className="hidden lg:inline">{t("browser.pageMode")}</span>
          </button>
          <button
            className={`inline-flex items-center gap-1 px-2 py-1 text-[10px] border-0 cursor-pointer transition-colors duration-150 ${active?.mode === "text" ? "bg-accent/12 text-accent" : "bg-transparent text-fg-faint hover:text-fg"}`}
            onClick={() => switchMode("text")}
            title={t("browser.textMode")}
          >
            <FileText size={11} />
            <span className="hidden lg:inline">{t("browser.textMode")}</span>
          </button>
        </div>
      </div>

      {/* 内容区 */}
      <div className="flex-1 min-h-0 relative">
        {active?.pendingHost ? (
          <div className="absolute inset-0 flex items-center justify-center p-6">
            <div className="w-full max-w-sm flex flex-col gap-3 p-5 rounded-xl border border-accent/25 bg-bg-elev shadow-[var(--ds-shadow-dropdown)]">
              <div className="flex items-center gap-2 text-accent">
                <ShieldAlert size={18} />
                <div className="text-sm font-medium">{t("browser.permissionTitle") ?? "访问确认"}</div>
              </div>
              <div className="text-xs leading-relaxed text-fg-dim">
                {t("browser.permissionBody", { host: active.pendingHost }) ?? `是否允许在应用内访问 ${active.pendingHost}？确认后该网站将被加入白名单。`}
              </div>
              <div className="flex items-center justify-end gap-2 mt-1">
                <button
                  className="inline-flex items-center px-3 py-1.5 rounded-md border border-border-soft bg-transparent text-fg-dim text-xs cursor-pointer transition-colors duration-150 hover:bg-bg-soft hover:text-fg"
                  onClick={() => onDeny(active.id)}
                >
                  {t("browser.deny") ?? "拒绝"}
                </button>
                <button
                  className="inline-flex items-center px-3 py-1.5 rounded-md border-0 bg-accent text-accent-fg text-xs cursor-pointer transition-all duration-150 hover:brightness-110 active:scale-95"
                  onClick={() => onAllow(active.pendingHost!, active.id)}
                >
                  {t("browser.allow") ?? "允许"}
                </button>
              </div>
            </div>
          </div>
        ) : empty ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-6 p-6 overflow-y-auto">
            <div className="flex flex-col items-center gap-2 text-fg-faint/50">
              <Globe size={30} />
              <div className="text-xs">{t("browser.emptyHint")}</div>
            </div>
            {recent.length > 0 && (
              <div className="w-full max-w-sm flex flex-col gap-2">
                <div className="text-[10px] uppercase tracking-wider text-fg-faint">
                  {t("browser.recent") ?? "最近访问"}
                </div>
                {recent.map((r) => (
                  <button
                    key={r.host}
                    className="flex items-center gap-2.5 px-3 py-2 rounded-md border border-border-soft/60 bg-bg-soft/40 text-left cursor-pointer transition-colors duration-150 hover:border-accent/40 hover:text-fg"
                    onClick={() => navigate(r.url)}
                  >
                    <span className="inline-flex items-center justify-center w-5 h-5 rounded text-[10px] font-bold bg-accent/15 text-accent shrink-0">
                      {hostInitial(r.url)}
                    </span>
                    <span className="flex-1 min-w-0">
                      <span className="block text-xs text-fg truncate">{r.host}</span>
                      <span className="block text-[10px] text-fg-faint font-mono truncate">{r.url}</span>
                    </span>
                  </button>
                ))}
              </div>
            )}
            <div className="flex items-center gap-4 text-[10px] text-fg-faint/70">
              <span><kbd>Ctrl</kbd>+<kbd>T</kbd> {t("browser.shortcutNewTab") ?? "新标签"}</span>
              <span><kbd>Ctrl</kbd>+<kbd>W</kbd> {t("browser.shortcutClose") ?? "关闭"}</span>
              <span><kbd>Ctrl</kbd>+<kbd>L</kbd> {t("browser.shortcutAddress") ?? "地址栏"}</span>
            </div>
          </div>
        ) : active.mode === "page" ? (
          <iframe
            key={`${current}#${active.refreshTick}`}
            src={current}
            className="w-full h-full border-0 bg-white"
            sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-downloads"
            referrerPolicy="no-referrer"
            onLoad={() => setTabs((ts) => updateTab(ts, active.id, { iframeLoading: false }))}
            title={current}
          />
        ) : active.textLoading ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-fg-faint/60">
            <Loader2 size={22} className="animate-spin text-accent" />
            <div className="text-xs font-mono">{t("browser.loading")}</div>
          </div>
        ) : active.textError ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 p-6 text-center">
            <div className="text-xs text-err font-mono break-all">{active.textError}</div>
          </div>
        ) : (
          <pre className="h-full overflow-auto p-3 text-[11.5px] leading-relaxed text-fg-dim font-mono whitespace-pre-wrap break-words">
            <span onMouseUp={detectSelection} onKeyUp={detectSelection}>
              {active.textContent}
            </span>
          </pre>
        )}

        {active?.mode === "text" && selection !== "" && onSendText && (
          <button
            className="absolute bottom-3 right-3 z-10 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md border-0 bg-accent text-accent-fg text-[11px] cursor-pointer shadow-[var(--ds-shadow-dropdown)] transition-all duration-150 hover:brightness-110 active:scale-95"
            onClick={sendSelection}
            title={t("browser.sendSelection") ?? "发送给 AI 分析"}
          >
            <Send size={11} />
            {t("browser.sendSelection") ?? "发送给 AI 分析"}
          </button>
        )}

        {active?.mode === "page" && active.iframeLoading && !active.pendingHost && (
          <div className="absolute top-1 left-1/2 -translate-x-1/2 z-10 flex items-center gap-1.5 px-2.5 py-1 rounded-full border border-border-soft bg-bg-elev text-fg-faint text-[10px] shadow-[var(--ds-shadow-dropdown)]">
            <Loader2 size={11} className="animate-spin text-accent" />
            <span className="font-mono">{t("browser.loading")}</span>
          </div>
        )}
      </div>
    </div>
  );
}
