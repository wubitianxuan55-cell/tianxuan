import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import {
  BarChart3, SquarePen, Brain, Blocks, ChevronDown, Cpu, FileText, FolderGit2, FolderTree,
  Settings as SettingsIcon, MessageSquare,
  PanelLeftClose, PanelLeftOpen, X,
} from "lucide-react";
import logo from "./assets/logo.png";
import { useT } from "./lib/i18n";
import { applyTheme } from "./lib/theme";
import { useController } from "./lib/store";
import { Transcript } from "./components/Transcript";
import { JumpBar } from "./components/JumpBar";
import { ToastProvider, useToast } from "./components/Toast";
import { Composer } from "./components/Composer";
import { TodoPanel } from "./components/TodoPanel";
import { ApprovalModal } from "./components/ApprovalModal";
import { AskCard } from "./components/AskCard";
import { ThemeSwitcher } from "./components/ThemeSwitcher";
import { StatusBar } from "./components/StatusBar";
import { ModelSwitcher } from "./components/ModelSwitcher";
const MemoryPanel = lazy(() => import("./components/MemoryPanel").then(m => ({ default: m.MemoryPanel })));
const HistoryPanel = lazy(() => import("./components/HistoryPanel").then(m => ({ default: m.HistoryPanel })));
const SettingsPanel = lazy(() => import("./components/SettingsPanel").then(m => ({ default: m.SettingsPanel })));
const CapabilitiesPanel = lazy(() => import("./components/CapabilitiesPanel").then(m => ({ default: m.CapabilitiesPanel })));
const PlanPanel = lazy(() => import("./components/PlanPanel").then(m => ({ default: m.PlanPanel })));
import { RuntimePanel } from "./components/RuntimePanel";
import { SkillsPanel } from "./components/SkillsPanel";
import { StatsPanel } from "./components/StatsPanel";
import { Skeleton } from "./components/Skeleton";
import { UpdateBanner } from "./components/UpdateBanner";
import { WorkspacePanel } from "./components/WorkspacePanel";
import { downloadMarkdown, exportAsMarkdown } from "./lib/export";
import type { MemoryView, SessionMeta } from "./lib/types";
import { usePlanExtractor } from "./hooks/usePlanExtractor";
import { useTodoExtractor } from "./hooks/useTodoExtractor";
import { useModeManager } from "./hooks/useModeManager";
import { useSessionManager } from "./hooks/useSessionManager";
import { useBridgeWatch } from "./hooks/useBridgeWatch";
import { useToolStats } from "./hooks/useToolStats";
import { useSidebar } from "./hooks/useSidebar";
import { useWorkspacePanel } from "./hooks/useWorkspacePanel";
import { CHAT_MIN_WIDTH, WORKSPACE_PANEL_MIN_WIDTH,
  SIDEBAR_DEFAULT_WIDTH, SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH,
  WORKSPACE_PANEL_DEFAULT_WIDTH, WORKSPACE_PANEL_MAX_WIDTH,
  WORKSPACE_FILE_TREE_PANEL_DEFAULT_WIDTH,
  WORKSPACE_FILE_TREE_PANEL_MIN_WIDTH, WORKSPACE_FILE_TREE_PANEL_MAX_WIDTH,
} from "./hooks/useLayoutSizes";
import CompactContext from "./hooks/useCompact";

function sessionTitle(session: SessionMeta, fallback: string): string {
  return session.title || session.preview || fallback;
}

function sessionTime(ms: number): string {
  return new Date(ms).toLocaleDateString([], { month: "short", day: "numeric" });
}

function NewSessionToast({ done }: { done: boolean }) {
  const toast = useToast();
  useEffect(() => { if (done) toast.show("新会话已创建", "info"); }, [done]);
  return null;
}

export default function App() {
  const {
    state,
    send,
    cancel,
    approve,
    answerQuestion,
    setPlan,
    setBypass,
    newSession,
    listSessions,
    resumeSession,
    deleteSession,
    renameSession,
    refreshMeta,
    pickWorkspace,
    switchWorkspace,
    rewind,
    setModel,
    fetchMemory,
    remember,
    forget,
    saveDoc,
  } = useController();
  const t = useT();
  const { mode, setMode, thinkLevel, themeNow, setTheme, switchingModel, cycleMode, handleThinkLevelChange, switchModel } = useModeManager(setPlan, setBypass, setModel);
  const [memView, setMemView] = useState<MemoryView | null>(null);
  const [histView, setHistView] = useState<SessionMeta[] | null>(null);
  const { sidebarSessions, sidebarQuery, setSidebarQuery, newSessionDone, refreshSessions, startNewSession, handleResumeSession, handleDeleteSession, handleRenameSession } = useSessionManager(newSession, listSessions, resumeSession, deleteSession, renameSession);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [capsOpen, setCapsOpen] = useState(false);
  const [showPlan, setShowPlan] = useState(false);
  const [rightTab, setRightTab] = useState<"files" | "runtime" | "skills" | "stats">("files");
  const [compactMode, setCompactMode] = useState(() => { try { return localStorage.getItem("tianxuan.compactMode") === "1"; } catch { return false; } });
  const [pendingPlanRevision, setPendingPlanRevision] = useState<string | null>(null);
  const [threadEl, setThreadEl] = useState<HTMLElement | null>(null);
  const [viewportWidth, setViewportWidth] = useState(() => (typeof window === "undefined" ? 1440 : window.innerWidth));

  const {
    sidebarCollapsed, sidebarWidth, sidebarResizing, effectiveSidebarWidth,
    sidebarWidthRef, sidebarExpandBlocked,
    toggleSidebar, setExpandedSidebarWidth, startSidebarResize,
    resizeSidebarWithKeyboard, handleWorkspacePreviewModeChange,
  } = useSidebar();

  const {
    workspacePanelOpen, workspacePanelResizing, workspacePanelMaximized,
    workspacePreviewModeActive, effectiveWorkspacePanelWidth,
    setWorkspacePanel, toggleWorkspacePanel,
    startWorkspacePanelResize, resizeWorkspacePanelWithKeyboard,
    setSavedWorkspacePanelWidth, setWorkspacePanelMaximized,
  } = useWorkspacePanel(effectiveSidebarWidth, viewportWidth);
  const { alive: bridgeAlive, onReconnect } = useBridgeWatch();
  useEffect(() => {
    onReconnect(() => { refreshMeta(); });
  }, [onReconnect, refreshMeta]);

  const { todoItem, todos, showTodos, setDismissedTodo } = useTodoExtractor(state.items);

  const planMarkdown = usePlanExtractor(state.items);

  useEffect(() => {
    if (!pendingPlanRevision || state.running) return;
    const text = pendingPlanRevision;
    setPendingPlanRevision(null);
    send(text);
  }, [pendingPlanRevision, send, state.running]);

  // Memory drawer: opening fetches a fresh snapshot; writes re-fetch so the
  // panel reflects what landed on disk.
  const openMemory = useCallback(async () => {
    setMemView(await fetchMemory());
  }, [fetchMemory]);

  const closeMemory = useCallback(() => setMemView(null), []);

  // handleSend intercepts the slash commands that need a desktop-native action
  // before they reach the backend: "/model <ref>" rebuilds on that model, and
  // "/memory" opens the memory drawer. Everything else — skills (/init, …),
  // custom commands, bare /model and the other read-only management verbs
  // (/skill, /hooks, /mcp) — goes straight to Submit, which the controller
  // resolves (a turn, or a listing Notice).
  const cwd = state.meta?.cwd;
  const cwdName = cwd ? cwd.split(/[/\\]/).filter(Boolean).pop() || cwd : "";

  const handleSend = useCallback(
    (displayText: string, submitText = displayText) => {
      const t = displayText.trim();
      const model = /^\/model\s+(\S+)$/.exec(t);
      if (model) {
        void switchModel(model[1]);
        return;
      }
      if (t === "/memory") {
        void openMemory();
        return;
      }
      send(t, submitText.trim());
    },
    [switchModel, openMemory, send],
  );

  const panelOpenRef = useRef(workspacePanelOpen);
  panelOpenRef.current = workspacePanelOpen;

  useEffect(() => {
    const onResize = () => {
      const w = window.innerWidth;
      setViewportWidth(w);
      const minForPanel = sidebarWidthRef.current + CHAT_MIN_WIDTH + WORKSPACE_PANEL_MIN_WIDTH;
      if (w < minForPanel && panelOpenRef.current) setWorkspacePanel(false);
    };
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  // 首次挂载时也检查窗口宽度，窄窗口自动关面板
  useEffect(() => {
    const minForPanel = effectiveSidebarWidth + CHAT_MIN_WIDTH + WORKSPACE_PANEL_MIN_WIDTH;
    if (window.innerWidth < minForPanel) setWorkspacePanel(false);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // 轮次结束时不再自动刷新侧边栏：refreshSessions 会改变 sidebarSessions
  // 引用，导致 useMemo 重算 currentSessionKey。若 session 路径恰好变化
  //（如首轮自动持久化），StatsPanel 会从空 localStorage key 加载数据，
  // 表现为统计面板全部清零。侧边栏列表由用户显式操作驱动刷新。
  useEffect(() => {
    // sidebar session list refresh is driven by explicit user actions
  }, [state.running, state.items.length, refreshSessions]);


  // History drawer: opening fetches the saved-session list; picking one resumes it
  // (the transcript swaps in; the model/folder are unchanged).
  const openHistory = useCallback(async () => {
    setHistView(await refreshSessions());
  }, [refreshSessions]);
  const closeHistory = useCallback(() => setHistView(null), []);
  const onResumeSession = useCallback(
    async (path: string) => { setHistView(null); await handleResumeSession(path); },
    [handleResumeSession],
  );
  const onDeleteSession = useCallback(
    async (path: string) => { await handleDeleteSession(path); setHistView(await refreshSessions()); },
    [handleDeleteSession, refreshSessions],
  );
  const onRenameSession = useCallback(
    async (path: string, title: string) => { await handleRenameSession(path, title); setHistView(await refreshSessions()); },
    [handleRenameSession, refreshSessions],
  );

  // Workspace: open the folder chooser and switch projects. The hook resets the
  // transcript and refreshes meta on a pick; refresh the sidebar sessions too so
  // the recent list belongs to the newly selected workspace. A cancel is a no-op.
  const switchFolder = useCallback(async (path?: string) => {
    const picked = path === undefined ? await pickWorkspace() : await switchWorkspace(path);
    if (picked) await refreshSessions();
    return picked;
  }, [pickWorkspace, switchWorkspace, refreshSessions]);

  const onRemember = useCallback(
    async (scope: string, note: string) => {
      await remember(scope, note);
      setMemView(await fetchMemory());
    },
    [remember, fetchMemory],
  );

  const onForget = useCallback(
    async (name: string) => {
      await forget(name);
      setMemView(await fetchMemory());
    },
    [forget, fetchMemory],
  );

  const onSaveDoc = useCallback(
    async (path: string, body: string) => {
      await saveDoc(path, body);
      setMemView(await fetchMemory());
    },
    [saveDoc, fetchMemory],
  );

  useEffect(() => { void refreshSessions(); }, [cwd, refreshSessions]);

  // 全局快捷键
  useEffect(() => {
    const onKey = (e: Event) => {
      const ke = e as globalThis.KeyboardEvent;
      const mod = ke.ctrlKey || ke.metaKey, t = ke.target as HTMLElement;
      const inInput = t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable;
      if (ke.key === "Escape" && !inInput && !state.running) {
        if (capsOpen) { ke.preventDefault(); setCapsOpen(false); return; }
        if (settingsOpen) { ke.preventDefault(); setSettingsOpen(false); return; }
        if (memView !== null) { ke.preventDefault(); setMemView(null); return; }
        if (histView !== null) { ke.preventDefault(); setHistView(null); return; }
        if (showPlan) { ke.preventDefault(); setShowPlan(false); return; }
        if (workspacePanelOpen) { ke.preventDefault(); setWorkspacePanel(false); return; }
        return;
      }
      if (!mod) return;
      if (ke.key === "n" && !state.running) { ke.preventDefault(); void startNewSession(); return; }
      if (ke.key === "k") { ke.preventDefault(); (document.querySelector("textarea[placeholder]") as HTMLTextAreaElement)?.focus(); return; }
      if (ke.key === ",") { ke.preventDefault(); setSettingsOpen(true); return; }
      if (ke.key === "M" && ke.shiftKey) { ke.preventDefault(); void openMemory(); return; }
      if (ke.key === "H" && ke.shiftKey) { ke.preventDefault(); void openHistory(); return; }
      if (ke.key === "b") { ke.preventDefault(); toggleSidebar(); return; }
      if (ke.key === "j") { ke.preventDefault(); toggleWorkspacePanel(); return; }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [state.running, capsOpen, settingsOpen, memView, histView, showPlan, workspacePanelOpen]);

  const sidebarToggleTitle = sidebarExpandBlocked
    ? t("sidebar.expandBlocked")
    : sidebarCollapsed
      ? t("sidebar.expand")
      : t("sidebar.collapse");

  const { toolCounts, skillCounts } = useToolStats(state.items);

  // 当前会话标识：直接使用 Go 后端生成的 .jsonl 文件路径作为 key。
  // 每个会话文件对应唯一的 localStorage key：新会话自然空数据开始，
  // 恢复/重启同一会话则统计数据持续累加，会话之间互不干扰。
  const currentSessionKey = useMemo(() => {
    const cur = sidebarSessions.find(s => s.current);
    return cur?.path
      ? cur.path.replace(/[\\/:*?"<>|]/g, "_")
      : cwd ? `unsaved_${cwd.replace(/[\\/:*?"<>|]/g, "_")}` : "unsaved";
  }, [sidebarSessions, cwd]);

  const layoutStyle = useMemo(
    () =>
      ({
        "--sidebar-expanded-width": `${sidebarWidth}px`,
        "--workspace-width": `${effectiveWorkspacePanelWidth}px`,
      }) as CSSProperties,
    [effectiveWorkspacePanelWidth, sidebarWidth],
  );
  return (
    <ToastProvider>
    <div className="app">
      <div
        className={[
          "layout",
          sidebarCollapsed ? "layout--sidebar-collapsed" : "",
          sidebarResizing ? "layout--resizing layout--sidebar-resizing" : "",
          workspacePanelOpen ? "layout--workspace-open" : "",
          workspacePanelResizing ? "layout--resizing layout--workspace-resizing" : "",
          workspacePanelOpen && workspacePanelMaximized ? "layout--workspace-maximized" : "",
        ]
          .filter(Boolean)
          .join(" ")}
        style={layoutStyle}
      >
        <aside
          className={`flex flex-col min-w-0 pt-[50px] pb-3 bg-sidebar-bg border-r border-border-soft select-none overflow-hidden drag-region ${
            sidebarCollapsed ? "items-center px-2" : "px-2.5"
          }`}
          aria-label="tianxuan navigation"
        >
          {/* Brand */}
          <div className={`flex items-center gap-2.5 px-2 pb-3.5 text-fg text-[15px] font-semibold ${
            sidebarCollapsed ? "flex-col gap-2 px-0 pb-3" : ""
          }`}>
            <img src={logo} alt="" className="w-6 h-6 rounded-md" />
            {!sidebarCollapsed && <span>tianxuan</span>}
            <button
              className={`inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-faint cursor-pointer transition-[color,background] duration-[0.12s] hover:text-fg hover:bg-sidebar-hover no-drag ${
                sidebarCollapsed ? "ml-0" : "ml-auto"
              } ${
                sidebarExpandBlocked ? "!text-fg-faint !bg-transparent !cursor-not-allowed opacity-55" : ""
              }`}
              onClick={sidebarExpandBlocked ? undefined : toggleSidebar}
              title={sidebarToggleTitle}
              aria-label={sidebarToggleTitle}
              aria-disabled={sidebarExpandBlocked}
            >
              {sidebarCollapsed ? <PanelLeftOpen size={15} /> : <PanelLeftClose size={15} />}
            </button>
          </div>

          {/* New session button */}
          <button
            className={`w-full min-w-0 border border-border rounded-lg bg-bg-elev text-fg font-medium cursor-pointer transition-[color,background,transform] duration-[0.12s] hover:bg-sidebar-hover hover:text-fg active:scale-[0.98] disabled:opacity-55 disabled:cursor-default flex items-center gap-2.5 h-9 px-2.5 mb-3 no-drag ${
              sidebarCollapsed ? "justify-center w-10 h-10 !rounded-[10px] !p-0 !gap-0" : ""
            }`}
            onClick={() => void startNewSession()}
            disabled={state.running}
            title={state.running ? t("common.busyHint") : t("topbar.newSession")}
          >
            <SquarePen size={15} />
            {!sidebarCollapsed && <span>{t("topbar.newSession")}</span>}
          </button>

          {/* Sessions section (hidden when collapsed) */}
          {!sidebarCollapsed && (
            <section className="flex-1 min-h-0 flex flex-col">
              <div className="flex items-center gap-2 px-1 pb-2 pl-2.5">
                <div className="flex-1 min-w-0 text-fg-faint font-mono text-[11px] uppercase tracking-wider">{t("sidebar.conversations")}</div>
                <button
                  className="shrink-0 border-0 rounded-md bg-transparent text-fg-faint text-[11.5px] px-1.5 py-0.5 cursor-pointer transition-[color,background,transform] duration-[0.12s] hover:text-fg hover:bg-sidebar-hover active:scale-[0.97] disabled:opacity-50 disabled:cursor-default disabled:hover:text-fg-faint disabled:hover:bg-transparent"
                  onClick={() => void openHistory()}
                  disabled={state.running}
                  title={state.running ? t("common.busyHint") : t("topbar.history")}
                >
                  {t("sidebar.viewAll")}
                </button>
              </div>
              {sidebarSessions.length > 0 && <input className="w-full bg-bg-soft border border-border-soft rounded-[5px] text-fg text-xs py-1 px-2 mb-2 outline-none focus:border-accent no-drag" placeholder={t("sidebar.search")} value={sidebarQuery} onChange={e => setSidebarQuery(e.target.value)} onKeyDown={e => e.stopPropagation()} />}
              <div className="min-h-0 overflow-y-auto pr-0.5">
                {(() => {
                  const q = sidebarQuery.trim().toLowerCase();
                  const visible = q ? sidebarSessions.filter((s: SessionMeta) => (s.title||s.preview||"").toLowerCase().includes(q)||s.path.toLowerCase().includes(q)) : sidebarSessions;
                  if (sidebarSessions.length===0) return <div className="py-2 px-2.5 text-fg-faint text-xs">{t("sidebar.noRecent")}</div>;
                  if (visible.length===0 && q) return <div className="py-2 px-2.5 text-fg-faint text-xs">无匹配</div>;
                  return visible.map((session: SessionMeta) => (
                    <div
                      className={`flex items-start gap-1 py-1 pl-2.5 pr-1 mb-0.5 rounded-md hover:bg-sidebar-hover group ${
                        session.current ? "bg-sidebar-active border-l-[3px] border-accent pl-[8px]" : ""
                      }`}
                      key={session.path}
                    >
                      <button className="flex items-start gap-2.5 flex-1 min-w-0 bg-transparent border-0 text-inherit cursor-pointer py-1 text-left disabled:cursor-default" onClick={() => void onResumeSession(session.path)} disabled={state.running||session.current} title={session.path}>
                        <MessageSquare size={14} className={`shrink-0 mt-0.5 ${session.current ? "text-accent" : "text-fg-faint"}`} />
                        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                          <span className={`overflow-hidden text-ellipsis whitespace-nowrap text-fg-dim text-[12.5px] leading-[1.35] font-medium ${session.current ? "text-accent" : ""}`}>{sessionTitle(session, t("history.emptySession"))}</span>
                          <span className="text-fg-faint font-mono text-[10.5px]">{session.current?t("history.current"):sessionTime(session.modTime)}</span>
                        </span>
                      </button>
                      {!session.current && <button className="hidden group-hover:block bg-transparent border-0 text-fg-faint text-[15px] cursor-pointer px-1 py-0.5 rounded-[3px] mt-1 hover:text-err" title="删除" onClick={e=>{e.stopPropagation();void onDeleteSession(session.path);void refreshSessions();}}>×</button>}
                    </div>
                  ));
                })()}
              </div>
            </section>
          )}

          {/* Bottom nav */}
          <nav className={`flex flex-col gap-0.5 shrink-0 pt-2.5 pb-2 border-t border-border-soft ${
            sidebarCollapsed ? "items-center w-full !pt-0 !pb-3" : ""
          }`}>
            <button className={`flex items-center gap-2.5 h-8 px-2.5 rounded-md text-fg-faint text-[13px] no-drag transition-[color,background,transform] duration-[0.12s] hover:text-fg hover:bg-sidebar-hover active:scale-[0.97] ${sidebarCollapsed ? "justify-center w-10 !p-0 !gap-0" : ""}`} onClick={() => void openMemory()} title={t("topbar.memory")}>
              <Brain size={15} />
              {!sidebarCollapsed && <span>{t("topbar.memory")}</span>}
            </button>
            <button
              className={`flex items-center gap-2.5 h-8 px-2.5 rounded-md text-fg-faint text-[13px] no-drag transition-[color,background,transform] duration-[0.12s] hover:text-fg hover:bg-sidebar-hover active:scale-[0.97] ${
                sidebarCollapsed ? "justify-center w-10 !p-0 !gap-0" : ""
              } ${showPlan ? "text-accent bg-accent-soft hover:bg-accent-soft" : ""}`}
              onClick={() => setShowPlan((v) => !v)}
              title={t("plan.title")}
            >
              <FileText size={15} />
              {!sidebarCollapsed && <span>{t("plan.title")}</span>}
            </button>
            <button className={`flex items-center gap-2.5 h-8 px-2.5 rounded-md text-fg-faint text-[13px] no-drag transition-[color,background,transform] duration-[0.12s] hover:text-fg hover:bg-sidebar-hover active:scale-[0.97] ${sidebarCollapsed ? "justify-center w-10 !p-0 !gap-0" : ""}`} onClick={() => setCapsOpen(true)} title={t("caps.title")}>
              <Blocks size={15} />
              {!sidebarCollapsed && <span>{t("caps.title")}</span>}
            </button>
            <button
              className={`flex items-center gap-2.5 h-8 px-2.5 rounded-md text-fg-faint text-[13px] no-drag transition-[color,background,transform] duration-[0.12s] hover:text-fg hover:bg-sidebar-hover active:scale-[0.97] disabled:opacity-40 disabled:cursor-default ${sidebarCollapsed ? "justify-center w-10 !p-0 !gap-0" : ""}`}
              onClick={() => setSettingsOpen(true)}
              disabled={state.running}
              title={state.running ? t("common.busyHint") : t("topbar.settings")}
            >
              <SettingsIcon size={15} />
              {!sidebarCollapsed && <span>{t("topbar.settings")}</span>}
            </button>
          </nav>
        </aside>
        <button
          className="sidebar-resizer"
          type="button"
          role="separator"
          aria-orientation="vertical"
          aria-label={t("sidebar.resize")}
          aria-valuemin={SIDEBAR_MIN_WIDTH}
          aria-valuemax={SIDEBAR_MAX_WIDTH}
          aria-valuenow={sidebarWidth}
          onPointerDown={startSidebarResize}
          onKeyDown={resizeSidebarWithKeyboard}
          onDoubleClick={() => setExpandedSidebarWidth(SIDEBAR_DEFAULT_WIDTH)}
          title={t("sidebar.resize")}
        />

        <section className="chat-pane">
          <header className="flex flex-shrink-0 items-center gap-3 px-12 bg-bg border-b border-border-soft shadow-[0_1px_3px_rgba(0,0,0,0.06)] select-none drag-region transition-all duration-200">
            <div className="flex items-center gap-2 min-w-0">
              <ModelSwitcher label={state.meta?.label ?? t("status.connecting")} onPick={switchModel} />
            </div>
            <div className="flex items-center gap-2 px-3">
              {cwd && (<button className="inline-flex items-center gap-[5px] h-[26px] px-[11px] border border-border bg-bg-soft text-fg-dim text-xs rounded-[7px] cursor-pointer transition-[color,border-color,background,transform] duration-[0.12s] hover:text-fg hover:border-fg-faint active:scale-[0.97] disabled:opacity-40 disabled:cursor-default disabled:hover:text-fg-dim disabled:hover:border-border no-drag flex items-center gap-1.5 text-fg-dim text-xs py-0.5 px-2" onClick={() => void switchFolder()} disabled={state.running}><FolderGit2 size={13} /><span>{cwdName}</span><ChevronDown size={11} /></button>)}
              <span className="flex items-center gap-0 border border-border-soft rounded-[5px] overflow-hidden no-drag">
                {(["fast", "normal", "deep"] as const).map(level => (
                  <button
                    key={level}
                    className={`bg-transparent border-0 border-r border-border-soft text-fg-faint text-[11px] px-2 py-0.5 cursor-pointer leading-tight no-drag last:border-r-0 hover:text-fg-dim hover:bg-bg-soft disabled:opacity-40 disabled:cursor-default ${
                      thinkLevel === level ? "text-accent bg-accent-soft" : ""
                    }`}
                    onClick={() => handleThinkLevelChange(level)}
                    disabled={state.running}
                    title={level === "fast" ? "快速思考" : level === "normal" ? "标准思考" : "深度思考"}
                  >
                    {level === "fast" ? "快速" : level === "normal" ? "标准" : "深度"}
                  </button>
                ))}
              </span>
            </div>
            <div className="flex-1" />
            <div className="flex items-center gap-2">
              <button className="inline-flex items-center gap-[5px] h-[26px] px-[11px] border border-border bg-bg-soft text-fg-dim text-xs rounded-[7px] cursor-pointer transition-[color,border-color,background,transform] duration-[0.12s] hover:text-fg hover:border-fg-faint active:scale-[0.97] no-drag" onClick={() => { const v = !compactMode; setCompactMode(v); try { localStorage.setItem("tianxuan.compactMode", v ? "1" : "0"); } catch {} }} title={compactMode ? "展开模式" : "紧凑模式"}>{compactMode ? "⊞" : "⊟"}</button>
              <button className="inline-flex items-center gap-[5px] h-[26px] px-[11px] border border-border bg-bg-soft text-fg-dim text-xs rounded-[7px] cursor-pointer transition-[color,border-color,background,transform] duration-[0.12s] hover:text-fg hover:border-fg-faint active:scale-[0.97] disabled:opacity-40 disabled:cursor-default disabled:hover:text-fg-dim disabled:hover:border-border no-drag" onClick={() => downloadMarkdown(exportAsMarkdown(state.items))} disabled={state.items.length===0}>导出</button>
              <button className="inline-flex items-center gap-[5px] h-[26px] px-[11px] border border-border bg-bg-soft text-fg-dim text-xs rounded-[7px] cursor-pointer transition-[color,border-color,background,transform] duration-[0.12s] hover:text-fg hover:border-fg-faint active:scale-[0.97] disabled:opacity-40 disabled:cursor-default disabled:hover:text-fg-dim disabled:hover:border-border no-drag" onClick={() => void startNewSession()} disabled={state.running||state.items.length===0}>清空</button>
              <ThemeSwitcher theme={themeNow} onSet={applyTheme} onStore={setTheme} />
            </div>
          </header>

          {state.meta?.startupErr && (
            <div className="shrink-0 px-4 py-2 text-[12.5px] bg-del-bg text-err border-b border-border-soft">{t("topbar.startupError", { msg: state.meta.startupErr })}</div>
          )}

          <UpdateBanner />
          <NewSessionToast done={newSessionDone} />
          <main className="main">
            <CompactContext.Provider value={compactMode}>
            {(state.meta?.ready === false && !state.meta?.startupErr) || switchingModel ? (
              <Skeleton />
            ) : (
              <>
                <Transcript items={state.items} onPrompt={send} onRewind={rewind} running={state.running} onThreadEl={setThreadEl} cwd={state.meta?.cwd} cwdName={cwdName} sessions={sidebarSessions} onResumeSession={handleResumeSession} meta={state.meta} />
                {state.items.length > 1 && <JumpBar items={state.items} threadEl={threadEl} />}
              </>
            )}
            </CompactContext.Provider>
          </main>

          <footer className={`shrink-0 border-t border-border-soft bg-bg px-8 ${compactMode ? "pt-2 pb-0.5" : "pt-3 pb-1"}`}>
            <CompactContext.Provider value={compactMode}>
            {showTodos && <TodoPanel todos={todos} onDismiss={() => setDismissedTodo(todoItem!.id)} />}
            <Composer
              running={state.running}
              mode={mode}
              cwd={state.meta?.cwd}
              onSend={handleSend}
              onCancel={cancel}
              onCycleMode={cycleMode}
              onPickFolder={switchFolder}
              disabled={state.meta?.ready === false || state.approval != null}
            />
            <StatusBar
              context={state.context}
              usage={state.usage}
              balance={state.balance}
              jobs={state.jobs}
              running={state.running}
              mode={mode}
              bridgeAlive={bridgeAlive}
              turnStartAt={state.turnStartAt}
              turnTokens={state.turnTokens}
              sessionTotal={state.sessionTotal}
              model={state.meta?.label}
              onOpenStats={() => { setRightTab("stats"); setWorkspacePanel(true); }}
            />
            </CompactContext.Provider>
          </footer>
        </section>

        {workspacePanelOpen && !workspacePanelMaximized && (
          <button
            className="workspace-panel-resizer"
            type="button"
            role="separator"
            aria-orientation="vertical"
            aria-label={t("workspace.resizePanel")}
            aria-valuemin={workspacePreviewModeActive ? WORKSPACE_PANEL_MIN_WIDTH : WORKSPACE_FILE_TREE_PANEL_MIN_WIDTH}
            aria-valuemax={workspacePreviewModeActive ? WORKSPACE_PANEL_MAX_WIDTH : WORKSPACE_FILE_TREE_PANEL_MAX_WIDTH}
            aria-valuenow={effectiveWorkspacePanelWidth}
            onPointerDown={startWorkspacePanelResize}
            onKeyDown={resizeWorkspacePanelWithKeyboard}
            onDoubleClick={() =>
              setSavedWorkspacePanelWidth(
                workspacePreviewModeActive ? WORKSPACE_PANEL_DEFAULT_WIDTH : WORKSPACE_FILE_TREE_PANEL_DEFAULT_WIDTH,
              )
            }
            title={t("workspace.resizePanel")}
          />
        )}

        {workspacePanelOpen && (
        <div className="flex flex-col min-w-0 overflow-hidden border-l border-border-soft bg-bg transition-all duration-200">
          <div className="flex items-center border-b border-border-soft overflow-hidden shrink">
            <button
              className={`flex items-center gap-[5px] px-3 py-2 text-xs bg-transparent border-0 border-b-2 cursor-pointer transition-[color,border-color] duration-[0.15s] hover:text-fg text-fg-dim border-transparent ${rightTab === "files" ? "text-accent border-accent" : ""}`}
              onClick={() => setRightTab("files")}
            >
              <FolderTree size={13} />
              <span>文件</span>
            </button>
            <button
              className={`flex items-center gap-[5px] px-3 py-2 text-xs bg-transparent border-0 border-b-2 cursor-pointer transition-[color,border-color] duration-[0.15s] hover:text-fg text-fg-dim border-transparent ${rightTab === "runtime" ? "text-accent border-accent" : ""}`}
              onClick={() => setRightTab("runtime")}
            >
              <Cpu size={13} />
              <span>工具</span>
            </button>
            <button
              className={`flex items-center gap-[5px] px-3 py-2 text-xs bg-transparent border-0 border-b-2 cursor-pointer transition-[color,border-color] duration-[0.15s] hover:text-fg text-fg-dim border-transparent ${rightTab === "skills" ? "text-accent border-accent" : ""}`}
              onClick={() => setRightTab("skills")}
            >
              <Blocks size={13} />
              <span>技能</span>
            </button>
            <button
              className={`flex items-center gap-[5px] px-3 py-2 text-xs bg-transparent border-0 border-b-2 cursor-pointer transition-[color,border-color] duration-[0.15s] hover:text-fg text-fg-dim border-transparent ${rightTab === "stats" ? "text-accent border-accent" : ""}`}
              onClick={() => setRightTab("stats")}
            >
              <BarChart3 size={13} />
              <span>统计</span>
            </button>
          </div>
          <div className="flex-1 min-h-0 overflow-y-auto">
            {rightTab === "files" ? (
              <WorkspacePanel
                open={workspacePanelOpen}
                cwd={state.meta?.cwd}
                maximized={workspacePanelMaximized}
                panelWidth={workspacePanelMaximized ? viewportWidth - effectiveSidebarWidth : effectiveWorkspacePanelWidth}
                onClose={() => setWorkspacePanel(false)}
                onToggleMaximized={() => setWorkspacePanelMaximized((value: boolean) => !value)}
                onPreviewModeChange={handleWorkspacePreviewModeChange}
              />
            ) : rightTab === "runtime" ? (
              <RuntimePanel counts={toolCounts} />
            ) : rightTab === "skills" ? (
              <SkillsPanel counts={skillCounts} />
            ) : null}
            {/* V5.25: StatsPanel 始终挂载（用 display:none 隐藏），
                确保在其他 tab 时也能接收 usage 事件并写入 localStorage。
                否则切换会话后打开统计面板，loadHistory 返回空数组。 */}
            <div style={{ display: rightTab === "stats" ? undefined : "none" }}>
              <StatsPanel usage={state.usage} perTurnUsage={state.perTurnUsage} turnSteps={state.turnSteps} context={state.context} model={state.meta?.label} sessionKey={currentSessionKey} toolCounts={toolCounts} skillCounts={skillCounts} />
            </div>
          </div>
        </div>
      )}

      {state.approval && (
          <ApprovalModal
            approval={state.approval}
            onAnswer={(allow, session) => {
              // Approving an exit_plan_mode plan leaves plan mode (the controller
              // flips the executor; mirror it here for the indicator).
              if (state.approval!.tool === "exit_plan_mode" && allow) setMode("normal");
              approve(state.approval!.id, allow, session);
            }}
            onRevisePlan={(text) => {
              setPendingPlanRevision(text);
              approve(state.approval!.id, false, false);
            }}
          />
        )}
      </div>

      {state.ask && (
        <AskCard
          ask={state.ask}
          onAnswer={answerQuestion}
          onDismiss={() => answerQuestion(state.ask!.id, [])}
        />
      )}

      <Suspense fallback={null}>
        {memView !== null && (
          <MemoryPanel
            view={memView}
            onClose={closeMemory}
            onRemember={onRemember}
            onForget={onForget}
            onSaveDoc={onSaveDoc}
          />
        )}
      </Suspense>

      <Suspense fallback={null}>
        {histView !== null && (
          <HistoryPanel
            sessions={histView}
            onResume={onResumeSession}
            onDelete={onDeleteSession}
            onRename={onRenameSession}
            onClose={closeHistory}
          />
        )}
      </Suspense>

      <Suspense fallback={null}>
        {settingsOpen && <SettingsPanel onClose={() => setSettingsOpen(false)} onChanged={() => void refreshMeta()} />}
      </Suspense>

      <Suspense fallback={null}>
        {capsOpen && <CapabilitiesPanel onClose={() => setCapsOpen(false)} />}
      </Suspense>

      <Suspense fallback={null}>
        {showPlan && (
          <PlanPanel
            planContent={planMarkdown}
            onClose={() => setShowPlan(false)}
          />
        )}
      </Suspense>
    </div>
    </ToastProvider>
  );
}
