import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import {
  BarChart3, SquarePen, Brain, Blocks, ChevronDown, Cpu, FolderGit2, FolderTree,
  Settings as SettingsIcon, MessageSquare,
} from "lucide-react";
import { Sidebar } from "./components/Sidebar";
import { useT } from "./lib/i18n";
import { sessionTitle, sessionTime } from "./lib/session";
import { applyTheme } from "./lib/theme";
import { useController } from "./lib/store";
import { app } from "./lib/bridge";
import { Transcript } from "./components/Transcript";
import { JumpBar } from "./components/JumpBar";
import { ToastProvider, useToast } from "./components/Toast";
import { Composer } from "./components/Composer";
import { TodoPanel } from "./components/TodoPanel";
import { ApprovalModal } from "./components/ApprovalModal";
import { AskCard } from "./components/AskCard";
import { ThemeSwitcher } from "./components/ThemeSwitcher";
import { ToolbarButton } from "./components/ToolbarButton";
import { StatusBar } from "./components/StatusBar";
import { ModelSwitcher } from "./components/ModelSwitcher";
const MemoryPanel = lazy(() => import("./components/MemoryPanel").then(m => ({ default: m.MemoryPanel })));
const HistoryPanel = lazy(() => import("./components/HistoryPanel").then(m => ({ default: m.HistoryPanel })));
const SettingsPanel = lazy(() => import("./components/SettingsPanel").then(m => ({ default: m.SettingsPanel })));
const CapabilitiesPanel = lazy(() => import("./components/CapabilitiesPanel").then(m => ({ default: m.CapabilitiesPanel })));
import { RuntimePanel } from "./components/RuntimePanel";
import { StartupSplash, shouldShowStartupSplash } from "./components/StartupSplash";
import { CommandPalette, type PaletteItem } from "./components/CommandPalette";
import { SkillsPanel } from "./components/SkillsPanel";
import { StatsPanel } from "./components/StatsPanel";
import { MessageNavigator } from "./components/MessageNavigator";
import { Skeleton } from "./components/Skeleton";
import { UpdateBanner } from "./components/UpdateBanner";
import { WorkspacePanel } from "./components/WorkspacePanel";
import { downloadMarkdown, exportAsMarkdown } from "./lib/export";
import type { MemorySuggestion, MemorySuggestionsView, MemoryView, SessionMeta, SkillSuggestion } from "./lib/types";
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
    setPermLevel: ctrlSetPermLevel,
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
    updateFact,
    changeFactType,
  } = useController();
  const t = useT();
  const { permLevel, setPermLevel, thinkLevel, themeNow, setTheme, switchingModel, handleThinkLevelChange, switchModel } = useModeManager(ctrlSetPermLevel, setModel);
  const [memView, setMemView] = useState<MemoryView | null>(null);
  const [histView, setHistView] = useState<SessionMeta[] | null>(null);
  const { sidebarSessions, sidebarQuery, setSidebarQuery, newSessionDone, refreshSessions, startNewSession, loadMore, hasMore, handleResumeSession, handleDeleteSession, handleRenameSession } = useSessionManager(newSession, listSessions, resumeSession, deleteSession, renameSession);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const newSessionAndReset = useCallback(async () => { setStatsReset(n => n + 1); await startNewSession(); }, [startNewSession]);
  const [statsReset, setStatsReset] = useState(0);
  const [capsOpen, setCapsOpen] = useState(false);
  const [rightTab, setRightTab] = useState<"files" | "runtime" | "skills" | "stats" | "messages">("files");
  const [pendingViewMode, setPendingViewMode] = useState<"files" | "changed" | null>(null);
  const [compactMode, setCompactMode] = useState(() => { try { return localStorage.getItem("tianxuan.compactMode") === "1"; } catch { return false; } });
  const [scrollToTurn, setScrollToTurn] = useState<((turn: number) => void) | null>(null);
  const [viewportWidth, setViewportWidth] = useState(() => (typeof window === "undefined" ? 1440 : window.innerWidth));
  const [splashDone, setSplashDone] = useState(!shouldShowStartupSplash());
  const splashHold = !(state.meta?.ready ?? false);
  const [paletteOpen, setPaletteOpen] = useState(false);

  const {
    sidebarCollapsed, sidebarWidth, sidebarResizing, effectiveSidebarWidth,
    sidebarWidthRef,
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



  // Memory drawer: opening fetches a fresh snapshot; writes re-fetch so the

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
  const reopenTimerRef = useRef<ReturnType<typeof setTimeout>>();

  // 清理 reopen timer
  useEffect(() => () => { if (reopenTimerRef.current) clearTimeout(reopenTimerRef.current); }, []);

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

  const onSaveFact = useCallback(
    async (name: string, body: string) => {
      await updateFact(name, body);
      setMemView(await fetchMemory());
    },
    [updateFact, fetchMemory],
  );

  const onAcceptMemorySuggestion = useCallback(
    async (candidate: MemorySuggestion) => {
      await app.AcceptMemorySuggestion(candidate);
      setMemView(await fetchMemory());
    },
    [fetchMemory],
  );

  const onAcceptSkillSuggestion = useCallback(
    async (candidate: SkillSuggestion) => {
      await app.AcceptSkillSuggestion(candidate);
    },
    [],
  );

  const onRefreshSuggestions = useCallback(async (): Promise<MemorySuggestionsView | null> => {
    try {
      return await app.MemorySuggestions();
    } catch {
      return null;
    }
  }, []);

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
        if (workspacePanelOpen) { ke.preventDefault(); setWorkspacePanel(false); return; }
        return;
      }
      if (!mod) return;
      if (ke.key === "n" && !state.running) { ke.preventDefault(); void newSessionAndReset(); return; }
      if (ke.key === "k") { ke.preventDefault(); setPaletteOpen(true); return; }
      if (ke.key === ",") { ke.preventDefault(); setSettingsOpen(true); return; }
      if (ke.key === "M" && ke.shiftKey) { ke.preventDefault(); void openMemory(); return; }
      if (ke.key === "H" && ke.shiftKey) { ke.preventDefault(); void openHistory(); return; }
      if (ke.key === "b") { ke.preventDefault(); toggleSidebar(); return; }
      if (ke.key === "j") { ke.preventDefault(); toggleWorkspacePanel(); return; }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [state.running, capsOpen, settingsOpen, memView, histView, workspacePanelOpen]);

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

  const paletteItems = useMemo<PaletteItem[]>(() => {
    const cmds: PaletteItem[] = [
      { id: "cmd-new", group: t("palette.group.commands") ?? "命令", title: t("topbar.newSession") ?? "新建会话", icon: <SquarePen size={15} />, compact: true, keywords: ["new", "新建"], run: () => void newSessionAndReset() },
      { id: "cmd-settings", group: t("palette.group.commands") ?? "命令", title: t("topbar.settings") ?? "设置", icon: <SettingsIcon size={15} />, compact: true, keywords: ["settings", "设置"], run: () => setSettingsOpen(true) },
      { id: "cmd-memory", group: t("palette.group.commands") ?? "命令", title: t("topbar.memory") ?? "记忆", icon: <Brain size={15} />, compact: true, keywords: ["memory", "记忆"], run: () => void openMemory() },
      { id: "cmd-history", group: t("palette.group.commands") ?? "命令", title: t("topbar.history") ?? "历史", icon: <MessageSquare size={15} />, compact: true, keywords: ["history", "历史"], run: () => void openHistory() },
      { id: "cmd-files", group: t("palette.group.commands") ?? "命令", title: "文件面板", icon: <FolderGit2 size={15} />, compact: true, keywords: ["files", "文件"], run: () => { setWorkspacePanel(true); setRightTab("files"); } },
      { id: "cmd-stats", group: t("palette.group.commands") ?? "命令", title: "统计面板", icon: <BarChart3 size={15} />, compact: true, keywords: ["stats", "统计"], run: () => { setWorkspacePanel(true); setRightTab("stats"); } },
    ];
    const sessionItems: PaletteItem[] = sidebarSessions.slice(0, 10).map((s) => ({
      id: `sess-${s.path}`,
      group: t("palette.group.sessions") ?? "会话",
      title: sessionTitle(s, t("history.emptySession") ?? "空会话"),
      hint: s.path,
      meta: sessionTime(s.modTime),
      badge: s.current ? "当前" : undefined,
      icon: <MessageSquare size={15} />,
      keywords: ["session", "会话"],
      run: () => { if (!s.current) void onResumeSession(s.path); },
    }));
    return [...cmds, ...sessionItems];
  }, [t, sidebarSessions, startNewSession, openMemory, openHistory, onResumeSession, setWorkspacePanel]);

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
    {!splashDone && <StartupSplash hold={splashHold} onDone={() => setSplashDone(true)} />}
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
        <Sidebar
          collapsed={sidebarCollapsed}
          toggleSidebar={toggleSidebar}
          running={state.running}
          newSessionAndReset={newSessionAndReset}
          sessions={sidebarSessions}
          searchQuery={sidebarQuery}
          onSearchChange={setSidebarQuery}
          hasMore={hasMore}
          onLoadMore={loadMore}
          onResumeSession={onResumeSession}
          onDeleteSession={onDeleteSession}
          onRenameSession={handleRenameSession}
          onOpenHistory={openHistory}
          onOpenMemory={openMemory}
          onOpenCaps={() => setCapsOpen(true)}
          onOpenSettings={() => setSettingsOpen(true)}
          startResize={startSidebarResize}
          resizeWithKeyboard={resizeSidebarWithKeyboard}
          onDoubleClickResize={() => setExpandedSidebarWidth(SIDEBAR_DEFAULT_WIDTH)}
          sidebarWidth={sidebarWidth}
          SIDEBAR_MIN_WIDTH={SIDEBAR_MIN_WIDTH}
          SIDEBAR_MAX_WIDTH={SIDEBAR_MAX_WIDTH}
        />

        <section className="chat-pane">
          <header className="flex flex-shrink-0 items-center gap-3 px-12 border-b border-border-soft select-none drag-region transition-all duration-200" style={{background: "var(--ds-gradient-topbar)", boxShadow: "var(--ds-shadow-topbar)"}}>
            <div className="flex items-center gap-2 min-w-0">
              <ModelSwitcher label={state.meta?.label ?? t("status.connecting")} onPick={switchModel} />
            </div>
            <div className="flex items-center gap-2 px-3">
              {cwd && (<button className="toolbar-btn no-drag" onClick={() => void switchFolder()} disabled={state.running}><FolderGit2 size={13} /><span>{cwdName}</span><ChevronDown size={11} /></button>)}
              <span className="flex items-center gap-0 border border-border-soft rounded-[5px] overflow-hidden no-drag">
                {(["fast", "normal", "deep"] as const).map(level => (
                  <button
                    key={level}
                    className={`bg-transparent border-0 border-r border-border-soft text-fg-faint text-[11px] px-2 py-0.5 cursor-pointer leading-tight no-drag last:border-r-0 hover:text-fg-dim hover:bg-bg-soft disabled:opacity-40 disabled:cursor-default transition-[color,background] duration-[var(--dur-fast)] ${
                      thinkLevel === level ? "text-accent font-semibold bg-accent/15 shadow-[inset_0_1px_2px_var(--accent-soft)]" : ""
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
              <ToolbarButton onClick={() => { const v = !compactMode; setCompactMode(v); try { localStorage.setItem("tianxuan.compactMode", v ? "1" : "0"); } catch {} }} title={compactMode ? "展开模式" : "紧凑模式"}>{compactMode ? "⊞" : "⊟"}</ToolbarButton>
              <ToolbarButton onClick={() => downloadMarkdown(exportAsMarkdown(state.items))} disabled={state.items.length===0}>导出</ToolbarButton>
              <ToolbarButton onClick={() => void newSessionAndReset()} disabled={state.running||state.items.length===0}>清空</ToolbarButton>
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
                <Transcript onPrompt={send} onRewind={rewind} running={state.running} onScrollToTurnReady={setScrollToTurn} cwd={state.meta?.cwd} cwdName={cwdName} sessions={sidebarSessions} onResumeSession={handleResumeSession} meta={state.meta} />
                {state.items.length > 1 && <JumpBar items={state.items} scrollToTurn={scrollToTurn ?? undefined} />}
              </>
            )}
            </CompactContext.Provider>
          </main>

          <footer className={`shrink-0 border-t border-border-soft bg-bg px-8 ${compactMode ? "pt-2 pb-0.5" : "pt-3 pb-1"}`}>
            <CompactContext.Provider value={compactMode}>
            {showTodos && <TodoPanel todos={todos} onDismiss={() => setDismissedTodo(todoItem!.id)} />}
            <div className="composer-glow">
            <Composer
              running={state.running}
              cwd={state.meta?.cwd}
              onSend={handleSend}
              onCancel={cancel}
              permLevel={permLevel}
              onSetPermLevel={setPermLevel}
              onPickFolder={switchFolder}
              disabled={state.meta?.ready === false || state.approval != null}
            />
            </div>
            <StatusBar
              context={state.context}
              usage={state.usage}
              balance={state.balance}
              jobs={state.jobs}
              running={state.running}
              bridgeAlive={bridgeAlive}
              turnStartAt={state.turnStartAt}
              turnTokens={state.turnTokens}
              sessionTotal={state.sessionTotal}
              model={state.meta?.label}
              permLevel={permLevel}
              onOpenChanges={useCallback(() => {
                setPendingViewMode("changed");
                if (workspacePanelOpen && rightTab === "files") {
                  setWorkspacePanel(false);
                  const id = setTimeout(() => setWorkspacePanel(true), 50);
                  // 防止快速双击堆积多个 timer
                  if (reopenTimerRef.current) clearTimeout(reopenTimerRef.current);
                  reopenTimerRef.current = id;
                } else {
                  setRightTab("files");
                  setWorkspacePanel(true);
                }
              }, [workspacePanelOpen, rightTab])}
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
              className={`flex items-center gap-1 px-3 py-2 text-xs bg-transparent border-0 border-b-2 cursor-pointer transition-[color,border-color] duration-[var(--dur-base)] hover:text-fg text-fg-dim border-transparent ${rightTab === "files" ? "text-accent border-accent" : ""}`}
              onClick={() => setRightTab("files")}
            >
              <FolderTree size={13} />
              <span>文件</span>
            </button>
            <button
              className={`flex items-center gap-1 px-3 py-2 text-xs bg-transparent border-0 border-b-2 cursor-pointer transition-[color,border-color] duration-[var(--dur-base)] hover:text-fg text-fg-dim border-transparent ${rightTab === "runtime" ? "text-accent border-accent" : ""}`}
              onClick={() => setRightTab("runtime")}
            >
              <Cpu size={13} />
              <span>工具</span>
            </button>
            <button
              className={`flex items-center gap-1 px-3 py-2 text-xs bg-transparent border-0 border-b-2 cursor-pointer transition-[color,border-color] duration-[var(--dur-base)] hover:text-fg text-fg-dim border-transparent ${rightTab === "skills" ? "text-accent border-accent" : ""}`}
              onClick={() => setRightTab("skills")}
            >
              <Blocks size={13} />
              <span>技能</span>
            </button>
            <button
              className={`flex items-center gap-1 px-3 py-2 text-xs bg-transparent border-0 border-b-2 cursor-pointer transition-[color,border-color] duration-[var(--dur-base)] hover:text-fg text-fg-dim border-transparent ${rightTab === "stats" ? "text-accent border-accent" : ""}`}
              onClick={() => setRightTab("stats")}
            >
              <BarChart3 size={13} />
              <span>统计</span>
            </button>
            <button
              className={`flex items-center gap-1 px-3 py-2 text-xs bg-transparent border-0 border-b-2 cursor-pointer transition-[color,border-color] duration-[var(--dur-base)] hover:text-fg text-fg-dim border-transparent ${rightTab === "messages" ? "text-accent border-accent" : ""}`}
              onClick={() => setRightTab("messages")}
            >
              <MessageSquare size={13} />
              <span>消息</span>
            </button>
          </div>
          <div className="flex-1 min-h-0 overflow-y-auto">
            {rightTab === "files" ? (
              <WorkspacePanel
                open={workspacePanelOpen}
                cwd={state.meta?.cwd}
                maximized={workspacePanelMaximized}
                panelWidth={workspacePanelMaximized ? viewportWidth - effectiveSidebarWidth : effectiveWorkspacePanelWidth}
                onClose={() => { setWorkspacePanel(false); setPendingViewMode(null); }}
                onToggleMaximized={() => setWorkspacePanelMaximized((value: boolean) => !value)}
                onPreviewModeChange={handleWorkspacePreviewModeChange}
                initialViewMode={pendingViewMode ?? undefined}
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
              <StatsPanel usage={state.usage} perTurnUsage={state.perTurnUsage} turnSteps={state.turnSteps} context={state.context} model={state.meta?.label} sessionKey={currentSessionKey} resetKey={statsReset} toolCounts={toolCounts} skillCounts={skillCounts} />
            </div>
            {rightTab === "messages" && (
              <MessageNavigator items={state.items} scrollToTurn={scrollToTurn ?? undefined} />
            )}
          </div>
        </div>
      )}
      </div>

      {state.approval && (
          <ApprovalModal
            approval={state.approval}
            onAnswer={(allow, session) => {
              approve(state.approval!.id, allow, session);
            }}
          />
        )}

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
            onSaveFact={onSaveFact}
            onChangeType={changeFactType}
            onAcceptMemorySuggestion={onAcceptMemorySuggestion}
            onAcceptSkillSuggestion={onAcceptSkillSuggestion}
            onRefreshSuggestions={onRefreshSuggestions}
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

      <CommandPalette
        open={paletteOpen}
        items={paletteItems}
        onClose={() => setPaletteOpen(false)}
      />
    </div>
    </ToastProvider>
  );
}
