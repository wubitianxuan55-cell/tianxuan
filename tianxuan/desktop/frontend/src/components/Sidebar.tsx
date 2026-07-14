import type { KeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import { useState, useEffect, useMemo } from "react";
import {
  SquarePen, Brain, MessageSquare,
  PanelLeftClose, PanelLeftOpen,
  Settings as SettingsIcon, CalendarDays, X,
} from "lucide-react";
import logo from "../assets/logo.png";
import { useT } from "../lib/i18n";
import { sessionTitle, sessionTime } from "../lib/session";
import type { SessionMeta, ContextInfo } from "../lib/types";
import { ContextBar } from "./StatusBar";

export interface SidebarProps {
  collapsed: boolean;
  toggleSidebar: () => void;
  running: boolean;
  newSessionAndReset: () => void;
  sessions: SessionMeta[];
  searchQuery: string;
  onSearchChange: (q: string) => void;
  hasMore: boolean;
  onLoadMore: () => void;
  onResumeSession: (path: string) => void;
  onDeleteSession: (path: string) => void;
  onRenameSession: (path: string, title: string) => void;
  onOpenHistory: () => void;
  onOpenMemory: () => void;
onOpenSettings: () => void;
  onOpenSchedule: () => void;
  context: ContextInfo;
  startResize: (e: ReactPointerEvent<HTMLButtonElement>) => void;
  resizeWithKeyboard: (e: KeyboardEvent<HTMLButtonElement>) => void;
  onDoubleClickResize: () => void;
  sidebarWidth: number;
  SIDEBAR_MIN_WIDTH: number;
  SIDEBAR_MAX_WIDTH: number;
}

export function Sidebar({
  collapsed,
  toggleSidebar,
  running,
  newSessionAndReset,
  sessions,
  searchQuery,
  onSearchChange,
  hasMore,
  onLoadMore,
  onResumeSession,
  onDeleteSession,
  onRenameSession,
  onOpenHistory,
  onOpenMemory,
onOpenSettings,
  onOpenSchedule,
  context,
  startResize,
  resizeWithKeyboard,
  onDoubleClickResize,
  sidebarWidth,
  SIDEBAR_MIN_WIDTH,
  SIDEBAR_MAX_WIDTH,
}: SidebarProps) {
  const t = useT();
  const toggleTitle = collapsed ? t("sidebar.expand") : t("sidebar.collapse");
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [renameTarget, setRenameTarget] = useState<string | null>(null);
  const [renameDraft, setRenameDraft] = useState("");
  // 互斥包装：两个编辑状态不能同时激活
  const startRename = (path: string, draft: string) => { setDeleteConfirm(null); setRenameTarget(path); setRenameDraft(draft); };
  const cancelEdit = () => { setRenameTarget(null); setDeleteConfirm(null); };
  const startDelete = (path: string) => { setRenameTarget(null); setDeleteConfirm(path); };
  // 搜索防抖：本地输入即时更新，200ms 后才同步到父组件触发过滤
  const [localQuery, setLocalQuery] = useState(searchQuery);
  useEffect(() => { setLocalQuery(searchQuery); }, [searchQuery]);
  useEffect(() => {
    const timer = setTimeout(() => onSearchChange(localQuery), 200);
    return () => clearTimeout(timer);
  }, [localQuery]);

  const q = localQuery.trim().toLowerCase();
  const visibleSessions = useMemo(() => q
    ? sessions.filter((s: SessionMeta) =>
        (s.title || s.preview || "").toLowerCase().includes(q) ||
        s.path.toLowerCase().includes(q)
      )
    : sessions, [sessions, q]);

  return (
    <>
      <aside
        className={`flex flex-col min-w-0 pt-[50px] pb-3 border-r border-border-soft select-none overflow-hidden drag-region ${
          collapsed ? "items-center px-2" : "px-2.5"
        }`}
        style={{ background: "var(--ds-gradient-sidebar)" }}
        aria-label="tianxuan navigation"
      >
        {/* Brand */}
        <div className={`flex items-center gap-2.5 px-2 pb-3.5 text-fg text-lg font-semibold ${
          collapsed ? "flex-col gap-2 px-0 pb-3" : ""
        }`}>
          <img src={logo} alt="" className="w-6 h-6 rounded-md" />
          {!collapsed && <span>tianxuan</span>}
          <button
            className={`inline-flex items-center justify-center w-7 h-7 border-0 rounded-md bg-transparent text-fg-faint cursor-pointer transition-[color,background] duration-150 hover:text-fg hover:bg-sidebar-hover focus-visible:ring-1 focus-visible:ring-accent/40 focus-visible:outline-none no-drag ${
              collapsed ? "ml-0" : "ml-auto"
            }`}
            onClick={toggleSidebar}
            title={toggleTitle}
            aria-label={toggleTitle}
          >
            {collapsed ? <PanelLeftOpen size={15} /> : <PanelLeftClose size={15} />}
          </button>
        </div>

        {/* New session button */}
        <button
          className={`w-full min-w-0 border-0 rounded-full bg-accent text-accent-fg font-semibold cursor-pointer transition-all duration-150 hover:brightness-110 active:scale-[0.97] disabled:opacity-40 disabled:cursor-default flex items-center gap-2 h-9 px-3 mb-3 no-drag ${
            collapsed ? "justify-center w-9 h-9 !rounded-full !p-0 !gap-0" : ""
          }`}
          style={{ boxShadow: "var(--ds-shadow-accent-btn)" }}
          onClick={() => void newSessionAndReset()}
          disabled={running}
          title={running ? t("common.busyHint") : t("topbar.newSession")}
        >
          <SquarePen size={15} />
          {!collapsed && <span>{t("topbar.newSession")}</span>}
        </button>

        {/* Collapsed session indicator */}
        {collapsed && (() => {
          const cur = sessions.find(s => s.current);
          return cur ? (
            <button
              className="w-9 h-9 mb-3 rounded-full bg-sidebar-active text-accent text-[12px] font-bold flex items-center justify-center cursor-pointer hover:bg-sidebar-hover transition-colors no-drag"
              title={sessionTitle(cur, "")}
              type="button"
            >
              {(cur.title || cur.preview || "?").charAt(0).toUpperCase()}
            </button>
          ) : null;
        })()}

        {/* Sessions section (hidden when collapsed) */}
        {!collapsed && (
          <section className="flex-1 min-h-0 flex flex-col">
            <div className="flex items-center gap-2 px-1 pb-2 pl-2.5">
              <div className="flex-1 min-w-0 text-fg-faint font-mono text-[11px] uppercase tracking-wider">
                {t("sidebar.conversations")}
              </div>
              <button
                className="shrink-0 border-0 rounded-md bg-transparent text-fg-faint text-[11px] px-1.5 py-0.5 cursor-pointer transition-[color,background,transform] duration-150 hover:text-fg hover:bg-sidebar-hover active:scale-[0.97] focus-visible:ring-1 focus-visible:ring-accent/40 focus-visible:outline-none disabled:opacity-50 disabled:cursor-default disabled:hover:text-fg-faint disabled:hover:bg-transparent"
                onClick={() => void onOpenHistory()}
                disabled={running}
                title={running ? t("common.busyHint") : t("topbar.history")}
              >
                {t("sidebar.viewAll")}
              </button>
            </div>
            <input
              className="w-full bg-bg-soft border border-border-soft rounded-[5px] text-fg text-xs py-1 px-2 mb-2 outline-none transition-colors duration-150 focus:border-accent no-drag"
              placeholder={t("sidebar.search")}
              value={localQuery}
              onChange={e => setLocalQuery(e.target.value)}
              onKeyDown={e => e.stopPropagation()}
            />
            <div className="min-h-0 overflow-y-auto pr-0.5">
              {sessions.length === 0
                  ? <div className="py-2 px-2.5 text-fg-faint text-xs">{t("sidebar.noRecent")}</div>
                : visibleSessions.length === 0 && q
                  ? <div className="py-2 px-2.5 text-fg-faint text-xs">无匹配</div>
                : visibleSessions.map((session: SessionMeta) => (
                  <div
                    className={`flex items-start gap-2 py-1 pl-2.5 pr-1 mb-0.5 rounded-md hover:bg-sidebar-hover group ${
                      session.current
                        ? "bg-sidebar-active border-l-[3px] border-accent pl-2 ring-1 ring-accent/35"
                        : ""
                    }`}
                    key={session.path}
                  >
                    <button
                      className="flex items-start gap-2.5 flex-1 min-w-0 bg-transparent border-0 text-inherit cursor-pointer py-1 text-left disabled:cursor-default"
                      onClick={() => void onResumeSession(session.path)}
                      disabled={running || session.current}
                      title={session.path}
                    >
                      <MessageSquare
                        size={14}
                        className={`shrink-0 mt-0.5 ${session.current ? "text-accent" : "text-fg-faint"}`}
                      />
                      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                        {renameTarget === session.path ? (
                          <input
                            className="w-full bg-bg border border-accent rounded px-1 py-0 text-fg text-sm outline-none"
                            value={renameDraft}
                            onChange={e => setRenameDraft(e.target.value)}
                            onKeyDown={e => {
                              if (e.key === "Enter") { e.preventDefault(); void onRenameSession(session.path, renameDraft.trim() || sessionTitle(session, "")); cancelEdit(); }
                              if (e.key === "Escape") { e.preventDefault(); cancelEdit(); }
                            }}
                            onBlur={() => { void onRenameSession(session.path, renameDraft.trim() || sessionTitle(session, "")); cancelEdit(); }}
                            autoFocus
                            onClick={e => e.stopPropagation()}
                          />
                        ) : (
                          <span
                            className={`overflow-hidden text-ellipsis whitespace-nowrap text-fg-dim text-sm leading-[1.35] font-medium cursor-text ${
                              session.current ? "text-accent" : ""
                            }`}
                            onDoubleClick={e => {
                              if (session.current) return;
                              e.stopPropagation();
                              startRename(session.path, sessionTitle(session, ""));
                            }}
                            title="双击重命名"
                          >
                            {sessionTitle(session, t("history.emptySession"))}
                          </span>
                        )}
                        <span className="text-fg-faint font-mono text-[10px]">
                          {session.current ? t("history.current") : sessionTime(session.modTime)}
                        </span>
                      </span>
                    </button>
                    {!session.current && (
                      deleteConfirm === session.path ? (
                        <span className="flex items-center gap-1 shrink-0">
                          <button className="bg-transparent border-0 text-[11px] text-err cursor-pointer px-1.5 py-0.5 rounded hover:bg-err/10 transition-colors duration-150" onClick={e => { e.stopPropagation(); void onDeleteSession(session.path); cancelEdit(); }}>
                            确认
                          </button>
                          <button className="bg-transparent border-0 text-[11px] text-fg-faint cursor-pointer px-1.5 py-0.5 rounded hover:bg-bg-soft transition-colors duration-150" onClick={e => { e.stopPropagation(); cancelEdit(); }}>
                            取消
                          </button>
                        </span>
                      ) : (
                        <button
                          className="hidden group-hover:flex bg-transparent border-0 text-fg-faint cursor-pointer p-0.5 rounded-[3px] mt-1 hover:text-err hover:bg-err/10 transition-colors duration-150 items-center justify-center"
                          title="删除"
                          onClick={e => { e.stopPropagation(); startDelete(session.path); }}
                        >
                          <X size={13} />
                        </button>
                      )
                    )}
                  </div>
                ))}
              {hasMore && !localQuery && (
                <button
                  className="w-full mt-1 py-1.5 text-fg-faint text-xs border border-border-soft rounded-md bg-transparent cursor-pointer hover:text-fg hover:bg-sidebar-hover transition-colors duration-150"
                  onClick={() => void onLoadMore()}
                  type="button"
                >
                  Show more...
                </button>
              )}
            </div>
          </section>
        )}

        {/* Context card — planner + executor token usage */}
        {!collapsed && (context.window > 0 || context.plannerWindow > 0) && (
          <div className="shrink-0 px-1 pb-1">
            <div className="rounded-lg border border-border-soft bg-bg/40 px-2.5 py-2.5 space-y-2">
              {context.plannerWindow > 0 && (
                <ContextBar label="规划" used={context.plannerUsed} window={context.plannerWindow} color="bg-purple-500/55" />
              )}
              {context.window > 0 && (
                <ContextBar label="执行" used={context.used} window={context.window} color="bg-cyan-500/55" />
              )}
            </div>
          </div>
        )}

        {/* Bottom nav */}
        <nav
          className={`flex flex-col gap-1 shrink-0 pt-2.5 pb-2 border-t border-border-soft ${
            collapsed ? "items-center w-full !pt-0 !pb-3" : ""
          }`}
        >
          <button
            className={`flex items-center gap-2.5 h-8 px-2.5 rounded-md text-fg-faint text-base no-drag transition-[color,background,transform] duration-150 hover:text-fg hover:bg-sidebar-hover active:scale-[0.97] focus-visible:ring-1 focus-visible:ring-accent/40 focus-visible:outline-none ${collapsed ? "justify-center w-10 !p-0 !gap-0" : ""}`}
            onClick={() => void onOpenMemory()}
            title={t("topbar.memory")}
          >
            <Brain size={15} />
            {!collapsed && <span>{t("topbar.memory")}</span>}
          </button>

          <button
            className={`flex items-center gap-2.5 h-8 px-2.5 rounded-md text-fg-faint text-base no-drag transition-[color,background,transform] duration-150 hover:text-fg hover:bg-sidebar-hover active:scale-[0.97] focus-visible:ring-1 focus-visible:ring-accent/40 focus-visible:outline-none ${collapsed ? "justify-center w-10 !p-0 !gap-0" : ""}`}
            onClick={() => onOpenSchedule()}
            title="定时任务"
          >
            <CalendarDays size={15} />
            {!collapsed && <span>定时任务</span>}
          </button>
          <button
            className={`flex items-center gap-2.5 h-8 px-2.5 rounded-md text-fg-faint text-[13px] no-drag transition-[color,background,transform] duration-[var(--dur-fast)] hover:text-fg hover:bg-sidebar-hover active:scale-[0.97] ${collapsed ? "justify-center w-10 !p-0 !gap-0" : ""}`}
            onClick={() => onOpenSettings()}
            title={t("topbar.settings")}
          >
            <SettingsIcon size={15} />
            {!collapsed && <span>{t("topbar.settings")}</span>}
          </button>
        </nav>
      </aside>

      {/* Resizer handle */}
      <button
        className="sidebar-resizer"
        type="button"
        role="separator"
        aria-orientation="vertical"
        aria-label={t("sidebar.resize")}
        aria-valuemin={SIDEBAR_MIN_WIDTH}
        aria-valuemax={SIDEBAR_MAX_WIDTH}
        aria-valuenow={sidebarWidth}
        onPointerDown={startResize}
        onKeyDown={resizeWithKeyboard}
        onDoubleClick={() => onDoubleClickResize()}
        title={t("sidebar.resize")}
      />
    </>
  );
}
