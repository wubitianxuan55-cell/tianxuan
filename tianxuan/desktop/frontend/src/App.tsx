import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import {
  BarChart3, SquarePen, Brain, Blocks, ChevronDown, Cpu, FolderGit2, FolderTree, GitBranch,
  Settings as SettingsIcon, MessageSquare,
} from "lucide-react";
import { Sidebar } from "./components/Sidebar";
import { useT } from "./lib/i18n";
import { sessionTitle, sessionTime } from "./lib/session";
import { applyColorScheme, applyThemeMode } from "./lib/theme";
import { useController } from "./lib/store";
import { app } from "./lib/bridge";
import { Transcript } from "./components/Transcript";
import { JumpBar } from "./components/JumpBar";
import { ToastProvider, useToast } from "./components/Toast";
import { Composer } from "./components/Composer";
import { TodoPanel } from "./components/TodoPanel";
import { ApprovalModal } from "./components/ApprovalModal";
import { AskCard } from "./components/AskCard";
import { PlanCard } from "./components/PlanCard";
import { ThemeSwitcher } from "./components/ThemeSwitcher";
import { ToolbarButton } from "./components/ToolbarButton";
import { StatusBar } from "./components/StatusBar";
import { ContextBar } from "./components/StatusBar";
import { ModelSwitcher } from "./components/ModelSwitcher";
import { ErrorBoundary } from "./components/ErrorBoundary";
const MemoryPanel = lazy(() => import("./components/MemoryPanel").then(m => ({ default: m.MemoryPanel })));
const HistoryPanel = lazy(() => import("./components/HistoryPanel").then(m => ({ default: m.HistoryPanel })));
const SettingsPanel = lazy(() => import("./components/SettingsPanel").then(m => ({ default: m.SettingsPanel })));
const CapabilitiesPanel = lazy(() => import("./components/CapabilitiesPanel").then(m => ({ default: m.CapabilitiesPanel })));
const SchedulePanel = lazy(() => import("./components/SchedulePanel").then(m => ({ default: m.SchedulePanel })));
import { RuntimePanel } from "./components/RuntimePanel";
import { StartupSplash, shouldShowStartupSplash } from "./components/StartupSplash";
import { CommandPalette, type PaletteItem } from "./components/CommandPalette";
import { SkillsPanel } from "./components/SkillsPanel";
import { StatsPanel, useStatsPersistence } from "./components/StatsPanel";
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
import { fmtTokens } from "./lib/stats";
import { useNow } from "./lib/useNow";

function NewSessionToast({ done }: { done: boolean }) {
  const toast = useToast();
  useEffect(() => { if (done) toast.show("新会话已创建", "info"); }, [done]);
  return null;
}

// ── RunStatus — 输入框上方的运行时状态行 ─────────────────────

function RunStatus({ running, turnStartAt, turnTokens, plannerLabel, phase }: {
  running: boolean;
  turnStartAt: number;
  turnTokens: number;
  plannerLabel?: string;
  phase: string; // "hermes" | "hephaestus" | ""
}) {
  const now = useNow();
  if (!running) return null;
  const elapsed = turnStartAt > 0 ? Math.max(0, now - Math.floor(turnStartAt / 1000)) : 0;
  const elapsedStr = elapsed < 60 ? `${elapsed}s` : `${Math.floor(elapsed / 60)}m${elapsed % 60}s`;
  const tokStr = turnTokens > 0 ? `↓${fmtTokens(turnTokens)}` : "";
  const isPlanner = phase === "hermes";
  const isExecutor = phase === "hephaestus";
  return (
    <div className="flex items-center justify-between px-4 py-1.5 text-[11px] select-none border-b border-border-soft/50 bg-bg-soft/30">
      <div className="flex items-center gap-2 text-fg-dim tabular-nums font-mono">
        <span className="font-medium">{elapsedStr}</span>
        {tokStr && <span className="text-fg-faint">{tokStr}</span>}
      </div>
      <div className="flex items-center gap-3">
        {plannerLabel && (
          <span className={`flex items-center gap-1.5 ${isPlanner ? "text-fg" : "text-fg-faint/60"}`}>
            <Brain size={12} className={isPlanner ? "text-purple-400" : ""} />
            <span className="font-medium">Hermes</span>
            <span>规划</span>
            {isPlanner && (
              <span className="inline-flex items-center gap-1 ml-0.5">
                <span className="w-1.5 h-1.5 rounded-full bg-purple-400 animate-pulse" />
                <span className="text-[10px] text-purple-400/70">中</span>
              </span>
            )}
          </span>
        )}
        <span className={`flex items-center gap-1.5 ${isExecutor ? "text-fg" : "text-fg-faint/60"}`}>
          <Cpu size={12} className={isExecutor ? "text-cyan-400" : ""} />
          <span className="font-medium">Hephaestus</span>
          <span>执行</span>
          {isExecutor && (
            <span className="inline-flex items-center gap-1 ml-0.5">
              <span className="w-1.5 h-1.5 rounded-full bg-cyan-400 animate-pulse" />
              <span className="text-[10px] text-cyan-400/70">中</span>
            </span>
          )}
        </span>
      </div>
    </div>
  );
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
  const { permLevel, setPermLevel, colorScheme, setColorScheme, themeMode, setThemeMode, switchingModel, switchModel } = useModeManager(ctrlSetPermLevel, setModel);
  const [memView, setMemView] = useState<MemoryView | null>(null);
  const [histView, setHistView] = useState<SessionMeta[] | null>(null);
  const { sidebarSessions, sidebarQuery, setSidebarQuery, newSessionDone, refreshSessions, startNewSession, loadMore, hasMore, handleResumeSession, handleDeleteSession, handleRenameSession } = useSessionManager(newSession, listSessions, resumeSession, deleteSession, renameSession);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const newSessionAndReset = useCallback(async () => { setStatsReset(n => n + 1); await startNewSession(); }, [startNewSession]);
  const [statsReset, setStatsReset] = useState(0);
  const [capsOpen, setCapsOpen] = useState(false);
  const [scheduleOpen, setScheduleOpen] = useState(false);
  const [rightTab, setRightTab] = useState<"files" | "runtime" | "skills" | "stats">("stats");
  const [pendingViewMode, setPendingViewMode] = useState<"files" | "changed" | null>(null);
  const [compactMode, setCompactMode] = useState(() => { try { return localStorage.getItem("tianxuan.compactMode") === "1"; } catch { return false; } });
  const [scrollToTurn, setScrollToTurn] = useState<((turn: number) => void) | null>(null);
  const [viewportWidth, setViewportWidth] = useState(() => (typeof window === "undefined" ? 1440 : window.innerWidth));
  const [splashDone, setSplashDone] = useState(!shouldShowStartupSplash());
  const splashHold = useMemo(() => !splashDone && !(state.meta?.ready ?? false), [splashDone, state.meta?.ready]);
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
        if (scheduleOpen) { ke.preventDefault(); setScheduleOpen(false); return; }
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
  }, [state.running, capsOpen, settingsOpen, memView, histView, scheduleOpen, workspacePanelOpen]);

  const { toolCounts, skillCounts } = useToolStats(state.items);

  // 当前会话标识：直接使用 Go 后端生成的 .jsonl 文件路径作为 key。
  // 每个会话文件对应唯一的 localStorage key：新会话自然空数据开始，
  // 恢复/重启同一会话则统计数据持续累加，会话之间互不干扰。
  const currentSessionPath = useMemo(
    () => sidebarSessions.find(s => s.current)?.path,
    [sidebarSessions],
  );
  const currentSessionKey = useMemo(() => {
    return currentSessionPath
      ? currentSessionPath.replace(/[\\/:*?"<>|]/g, "_")
      : cwd ? `unsaved_${cwd.replace(/[\\/:*?"<>|]/g, "_")}` : "unsaved";
  }, [currentSessionPath, cwd]);

  const statsPersistence = useStatsPersistence(currentSessionKey, statsReset, state.turnSteps, state.perTurnUsage);

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
  const activePhase = useMemo(() => {
    for (let i = state.items.length - 1; i >= 0; i--) {
      const item = state.items[i];
      if (item.kind === "phase") {
        const t = item.text.toLowerCase();
        if (t.includes("hermes")) return "hermes";
        if (t.includes("hephaestus")) return "hephaestus";
      }
    }
    return "";
  }, [state.items]);
  return (
    <ToastProvider>
    <ErrorBoundary>
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
          onOpenSchedule={() => setScheduleOpen(true)}
          startResize={startSidebarResize}
          resizeWithKeyboard={resizeSidebarWithKeyboard}
          onDoubleClickResize={() => setExpandedSidebarWidth(SIDEBAR_DEFAULT_WIDTH)}
          sidebarWidth={sidebarWidth}
          SIDEBAR_MIN_WIDTH={SIDEBAR_MIN_WIDTH}
          SIDEBAR_MAX_WIDTH={SIDEBAR_MAX_WIDTH}
        />

        <section className="chat-pane">
          <header className="flex flex-shrink-0 items-center gap-3 px-4 border-b border-border-soft select-none drag-region transition-all duration-200" style={{background: "var(--ds-gradient-topbar)", boxShadow: "var(--ds-shadow-topbar)"}}>
            <div className="flex items-center gap-2 min-w-0">
              <ModelSwitcher label={state.meta?.label ?? t("status.connecting")} onPick={switchModel} />
            </div>
            {/* 顶栏上下文用量 — 规划者(紫) + 执行者(青) */}
            {(state.context.window > 0 || state.context.plannerWindow > 0) && (
              <div className="flex items-center gap-4 flex-1 min-w-0">
                {state.context.plannerWindow > 0 && (
                  <ContextBar
                    label="规划"
                    used={state.context.plannerUsed}
                    window={state.context.plannerWindow}
                    color="bg-purple-500/55"
                  />
                )}
                {state.context.window > 0 && (
                  <ContextBar
                    label="执行"
                    used={state.context.used}
                    window={state.context.window}
                    color="bg-cyan-500/55"
                  />
                )}
              </div>
            )}
            <div className="flex items-center gap-2 px-3">
              {cwd && (<button className="toolbar-btn no-drag" onClick={() => void switchFolder()} disabled={state.running}><FolderGit2 size={13} /><span>{cwdName}</span><ChevronDown size={11} /></button>)}
            </div>
            <div className="flex-1" />
            <div className="flex items-center gap-2">
              <ToolbarButton onClick={() => {
                setPendingViewMode("changed");
                if (workspacePanelOpen && rightTab === "files") {
                  setWorkspacePanel(false);
                  const id = setTimeout(() => setWorkspacePanel(true), 50);
                  if (reopenTimerRef.current) clearTimeout(reopenTimerRef.current);
                  reopenTimerRef.current = id;
                } else {
                  setRightTab("files");
                  setWorkspacePanel(true);
                }
              }} title="查看文件变更"><GitBranch size={13} /></ToolbarButton>
              <ToolbarButton onClick={() => { const v = !compactMode; setCompactMode(v); try { localStorage.setItem("tianxuan.compactMode", v ? "1" : "0"); } catch {} }} title={compactMode ? "展开模式" : "紧凑模式"}>{compactMode ? "⊞" : "⊟"}</ToolbarButton>
              <ToolbarButton onClick={() => downloadMarkdown(exportAsMarkdown(state.items))} disabled={state.items.length===0}>导出</ToolbarButton>
              <ToolbarButton onClick={() => void newSessionAndReset()} disabled={state.running||state.items.length===0}>清空</ToolbarButton>
              <ThemeSwitcher scheme={colorScheme} mode={themeMode} onScheme={(s) => { applyColorScheme(s); setColorScheme(s); }} onMode={(m) => { applyThemeMode(m); setThemeMode(m); }} />
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

          <footer className={`shrink-0 border-t border-border-soft bg-bg px-4 ${compactMode ? "pt-2 pb-0.5" : "pt-3 pb-1"}`}>
            <CompactContext.Provider value={compactMode}>
            {showTodos && <TodoPanel todos={todos} onDismiss={() => setDismissedTodo(todoItem!.id)} />}
            <RunStatus
              running={state.running}
              turnStartAt={state.turnStartAt}
              turnTokens={state.turnTokens}
              plannerLabel={state.meta?.plannerLabel}
              phase={activePhase}
            />
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
              usage={state.usage}
              balance={state.balance}
              jobs={state.jobs}
              running={state.running}
              bridgeAlive={bridgeAlive}
              sessionTotal={state.sessionTotal}
              model={state.meta?.label}
              subagentModel={state.meta?.subagentLabel}
              permLevel={permLevel}
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
            {rightTab === "stats" && (
              <StatsPanel
                data={statsPersistence.data}
                clearData={statsPersistence.clearData}
                perTurnPlannerUsage={state.perTurnPlannerUsage}
                perTurnExecutorUsage={state.perTurnExecutorUsage}
                perTurnSubUsage={state.perTurnSubUsage}
                turnSteps={state.turnSteps}
                subagentModel={state.meta?.subagentLabel}
                toolCounts={toolCounts}
                skillCounts={skillCounts}
              />
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

      {state.ask && (state.ask.questions[0]?.plan ? (
        <PlanCard
          ask={state.ask}
          onAnswer={answerQuestion}
          onDismiss={() => answerQuestion(state.ask!.id, [])}
        />
      ) : (
        <AskCard
          ask={state.ask}
          onAnswer={answerQuestion}
          onDismiss={() => answerQuestion(state.ask!.id, [])}
        />
      ))}

      <Suspense fallback={null}>
        {memView !== null && (
          <MemoryPanel
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

      <Suspense fallback={null}>
        {scheduleOpen && <SchedulePanel onClose={() => setScheduleOpen(false)} />}
      </Suspense>

      <CommandPalette
        open={paletteOpen}
        items={paletteItems}
        onClose={() => setPaletteOpen(false)}
      />
    </div>
    </ErrorBoundary>
    </ToastProvider>
  );
}
