import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import {
  BarChart3, SquarePen, Brain, Blocks, ChevronDown, Cpu, FileText, FolderGit2, FolderTree,
  History, Settings as SettingsIcon, MessageSquare,
  PanelLeftClose, PanelLeftOpen,
} from "lucide-react";
import logo from "./assets/logo.png";
import { useT } from "./lib/i18n";
import { useController } from "./lib/store";
import { applyTheme } from "./lib/theme";
import type { Theme } from "./lib/theme";
import { Transcript } from "./components/Transcript";
import { Composer } from "./components/Composer";
import { TodoPanel } from "./components/TodoPanel";
import { ApprovalModal } from "./components/ApprovalModal";
import { AskCard } from "./components/AskCard";
import { StatusBar } from "./components/StatusBar";
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

function sessionTitle(session: SessionMeta, fallback: string): string {
  return session.title || session.preview || fallback;
}

function sessionTime(ms: number): string {
  return new Date(ms).toLocaleDateString([], { month: "short", day: "numeric" });
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
  const [pendingPlanRevision, setPendingPlanRevision] = useState<string | null>(null);
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

  useEffect(() => {
    if (!state.running && state.items.length > 0) void refreshSessions();
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
      if (ke.key === "k") { ke.preventDefault(); (document.querySelector(".composer__input") as HTMLTextAreaElement)?.focus(); return; }
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

  // 当前会话标识：用于统计面板按会话分组
  // V5.25: 加入 sessionNonce 确保每次新建/恢复会话生成唯一 key，
  // 修复新建会话和切换会话时统计面板不刷新的问题。
  const currentSessionKey = useMemo(() => {
    const cur = sidebarSessions.find(s => s.current);
    const base = cur?.path
      ? cur.path.replace(/[\\/:*?"<>|]/g, "_")
      : cwd ? `unsaved_${cwd.replace(/[\\/:*?"<>|]/g, "_")}` : "unsaved";
    return `${base}_${state.sessionNonce}`;
  }, [sidebarSessions, cwd, state.sessionNonce]);

  const layoutStyle = useMemo(
    () =>
      ({
        "--sidebar-expanded-width": `${sidebarWidth}px`,
        "--workspace-width": `${effectiveWorkspacePanelWidth}px`,
      }) as CSSProperties,
    [effectiveWorkspacePanelWidth, sidebarWidth],
  );
  return (
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
        <aside className={`sidebar${sidebarCollapsed ? " sidebar--collapsed" : ""}`} aria-label="tianxuan navigation">
          <div className="sidebar__brand">
            <img src={logo} alt="" className="sidebar__logo" />
            <span>tianxuan</span>
            <button
              className={`sidebar__toggle${sidebarExpandBlocked ? " sidebar__toggle--blocked" : ""}`}
              onClick={sidebarExpandBlocked ? undefined : toggleSidebar}
              title={sidebarToggleTitle}
              aria-label={sidebarToggleTitle}
              aria-disabled={sidebarExpandBlocked}
            >
              {sidebarCollapsed ? <PanelLeftOpen size={15} /> : <PanelLeftClose size={15} />}
            </button>
          </div>

          <button
            className="sidebar__new"
            onClick={() => void startNewSession()}
            disabled={state.running}
            title={state.running ? t("common.busyHint") : t("topbar.newSession")}
          >
            <SquarePen size={15} />
            <span>{t("topbar.newSession")}</span>
          </button>

          <section className="sidebar__section">
            <div className="sidebar__section-head">
              <div className="sidebar__section-title">{t("sidebar.conversations")}</div>
              <button
                className="sidebar__view-all"
                onClick={() => void openHistory()}
                disabled={state.running}
                title={state.running ? t("common.busyHint") : t("topbar.history")}
              >
                {t("sidebar.viewAll")}
              </button>
            </div>
            {sidebarSessions.length > 3 && <input className="sidebar__search" placeholder="搜索…" value={sidebarQuery} onChange={e => setSidebarQuery(e.target.value)} onKeyDown={e => e.stopPropagation()} />}
            <div className="sidebar__sessions">
              {(() => {
                const q = sidebarQuery.trim().toLowerCase();
                const visible = q ? sidebarSessions.filter((s: SessionMeta) => (s.title||s.preview||"").toLowerCase().includes(q)||s.path.toLowerCase().includes(q)) : sidebarSessions;
                if (sidebarSessions.length===0) return <div className="sidebar__empty">{t("sidebar.noRecent")}</div>;
                if (visible.length===0 && q) return <div className="sidebar__empty">无匹配</div>;
                return visible.map((session: SessionMeta) => (
                  <div className={`sidebar-session${session.current?" sidebar-session--current":""}`} key={session.path}>
                    <button className="sidebar-session__main" onClick={() => void onResumeSession(session.path)} disabled={state.running||session.current} title={session.path}>
                      <MessageSquare size={14} />
                      <span className="sidebar-session__body">
                        <span className="sidebar-session__title">{sessionTitle(session, t("history.emptySession"))}</span>
                        <span className="sidebar-session__meta">{session.current?t("history.current"):sessionTime(session.modTime)}</span>
                      </span>
                    </button>
                    {!session.current && <button className="sidebar-session__del" title="删除" onClick={e=>{e.stopPropagation();void onDeleteSession(session.path);void refreshSessions();}}>×</button>}
                  </div>
                ));
              })()}
            </div>
          </section>

          <nav className="sidebar__nav">
            <button
              className="sidebar__navitem sidebar__navitem--sessions"
              onClick={() => void openHistory()}
              disabled={state.running}
              title={state.running ? t("common.busyHint") : t("topbar.history")}
            >
              <History size={15} />
              <span>{t("topbar.history")}</span>
            </button>
            <button className="sidebar__navitem" onClick={() => void openMemory()} title={t("topbar.memory")}>
              <Brain size={15} />
              <span>{t("topbar.memory")}</span>
            </button>
            <button
              className={`sidebar__navitem ${showPlan ? "sidebar__navitem--active" : ""}`}
              onClick={() => setShowPlan((v) => !v)}
              title={t("plan.title")}
            >
              <FileText size={15} />
              <span>{t("plan.title")}</span>
            </button>
            <button className="sidebar__navitem" onClick={() => setCapsOpen(true)} title={t("caps.title")}>
              <Blocks size={15} />
              <span>{t("caps.title")}</span>
            </button>
            <button
              className="sidebar__navitem"
              onClick={() => setSettingsOpen(true)}
              disabled={state.running}
              title={state.running ? t("common.busyHint") : t("topbar.settings")}
            >
              <SettingsIcon size={15} />
              <span>{t("topbar.settings")}</span>
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
          <header className="topbar">
            <div className="topbar__identity">
              <span className="topbar__title">tianxuan</span>
              <span className="topbar__model">{state.meta?.label ?? "…"}</span>
            </div>
            <div className="topbar__center">
              {cwd && (<button className="chip topbar__workspace" onClick={() => void switchFolder()} disabled={state.running}><FolderGit2 size={13} /><span>{cwdName}</span><ChevronDown size={11} /></button>)}
              <span className="topbar__think">
                {(["fast", "normal", "deep"] as const).map(level => (
                  <button
                    key={level}
                    className={`topbar__think-btn${thinkLevel === level ? " topbar__think-btn--on" : ""}`}
                    onClick={() => handleThinkLevelChange(level)}
                    disabled={state.running}
                    title={level === "fast" ? "快速思考" : level === "normal" ? "标准思考" : "深度思考"}
                  >
                    {level === "fast" ? "⚡" : level === "normal" ? "🧠" : "💎"}
                  </button>
                ))}
              </span>
            </div>
            <div className="topbar__spacer" />
            <div className="topbar__actions">
              <button className="chip" onClick={() => downloadMarkdown(exportAsMarkdown(state.items))} disabled={state.items.length===0}>导出</button>
              <button className="chip" onClick={() => void startNewSession()} disabled={state.running||state.items.length===0}>清空</button>
              <button className="chip" onClick={() => { const themes:Theme[]=["dark","light","warm","ice"]; const cur=themeNow==="auto"?"dark":themeNow; const idx=themes.indexOf(cur); const n=themes[(idx+1)%4]; applyTheme(n); setTheme(n); }}>
                {themeNow==="dark"?"深色":themeNow==="light"?"浅色":themeNow==="warm"?"暖护眼":"冰蓝"}
              </button>
              <button
                className="chip chip--icon"
                onClick={() => void openHistory()}
                disabled={state.running}
                title={state.running ? t("common.busyHint") : t("topbar.history")}
              >
                <History size={13} />
              </button>

              <button
                className="chip chip--icon"
                onClick={() => void startNewSession()}
                disabled={state.running}
                title={state.running ? t("common.busyHint") : t("topbar.newSession")}
              >
                <SquarePen size={13} />
              </button>
            </div>
          </header>

          {state.meta?.startupErr && (
            <div className="banner banner--error">{t("topbar.startupError", { msg: state.meta.startupErr })}</div>
          )}

          <UpdateBanner />
          {newSessionDone && <div className="new-session-toast">新会话已创建</div>}
          <main className="main">
            {(state.meta?.ready === false && !state.meta?.startupErr) || switchingModel ? (
              <Skeleton />
            ) : (
              <Transcript items={state.items} onPrompt={send} onRewind={rewind} running={state.running} />
            )}
          </main>

          <footer className="footer">
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
              meta={state.meta}
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
              onSwitchModel={switchModel}
            />
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
        <div className="right-panel">
          <div className="right-panel__tabs">
            <button
              className={`right-panel__tab ${rightTab === "files" ? "right-panel__tab--active" : ""}`}
              onClick={() => setRightTab("files")}
            >
              <FolderTree size={13} />
              <span>文件</span>
            </button>
            <button
              className={`right-panel__tab ${rightTab === "runtime" ? "right-panel__tab--active" : ""}`}
              onClick={() => setRightTab("runtime")}
            >
              <Cpu size={13} />
              <span>工具</span>
            </button>
            <button
              className={`right-panel__tab ${rightTab === "skills" ? "right-panel__tab--active" : ""}`}
              onClick={() => setRightTab("skills")}
            >
              <Blocks size={13} />
              <span>技能</span>
            </button>
            <button
              className={`right-panel__tab ${rightTab === "stats" ? "right-panel__tab--active" : ""}`}
              onClick={() => setRightTab("stats")}
            >
              <BarChart3 size={13} />
              <span>统计</span>
            </button>
          </div>
          <div className="right-panel__body">
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
              <StatsPanel usage={state.usage} perTurnUsage={state.perTurnUsage} turnSteps={state.turnSteps} context={state.context} model={state.meta?.label} sessionKey={currentSessionKey} refreshNonce={state.sessionNonce} toolCounts={toolCounts} skillCounts={skillCounts} />
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
  );
}
