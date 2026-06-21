import { useMemo, useState } from "react";
import { Pencil, Search, Trash2, Check, X, MessageSquare, Clock } from "lucide-react";
import { t, useT } from "../lib/i18n";
import type { SessionMeta } from "../lib/types";
import { CloseButton } from "./CloseButton";
import { ResizableDrawer } from "./ResizableDrawer";

export function HistoryPanel({
  sessions,
  onResume,
  onDelete,
  onRename,
  onClose,
}: {
  sessions: SessionMeta[];
  onResume: (path: string) => void;
  onDelete: (path: string) => void;
  onRename: (path: string, title: string) => void;
  onClose: () => void;
}) {
  const tr = useT();
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [confirming, setConfirming] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return sessions;
    return sessions.filter((s) => {
      const text = [(s.title || s.preview || ""), s.path].join(" ").toLowerCase();
      return text.includes(q);
    });
  }, [sessions, query]);

  const startRename = (s: SessionMeta) => {
    setConfirming(null);
    setEditing(s.path);
    setDraft(s.title || s.preview || "");
  };
  const commitRename = (path: string) => {
    const title = draft.trim();
    if (title) onRename(path, title);
    setEditing(null);
  };

  // 按日期分组
  const groups: { label: string; items: SessionMeta[] }[] = [];
  for (const s of filtered) {
    const label = dayLabel(s.modTime);
    const last = groups[groups.length - 1];
    if (last && last.label === label) last.items.push(s);
    else groups.push({ label, items: [s] });
  }

  const hasSessions = sessions.length > 0;

  return (
    <ResizableDrawer onClose={onClose}>
      {/* ── Header ── */}
      <header className="flex items-center justify-between shrink-0 px-4 py-3.5 bg-bg-elev border-b border-border">
        <div className="flex items-center gap-2.5 min-w-0">
          <span className="text-[15px] font-semibold text-fg">{tr("history.title")}</span>
          {hasSessions && (
            <span className="text-fg-faint text-[11px] bg-bg-soft px-2 py-0.5 rounded-full font-mono">
              {sessions.length}
            </span>
          )}
        </div>
        <CloseButton onClick={onClose} />
      </header>

      {/* ── 搜索栏 ── */}
      <div className="shrink-0 px-4 py-3 border-b border-border-soft bg-bg-soft/30">
        <label className="flex items-center gap-1.5 px-2.5 h-8 border border-border rounded-md bg-bg text-fg-faint focus-within:border-accent focus-within:shadow-[0_0_0_2px_var(--accent-soft)] transition-[border-color,box-shadow] duration-[0.12s]">
          <Search size={14} />
          <input
            className="flex-1 border-0 outline-none bg-transparent text-fg text-[13px] placeholder:text-fg-faint"
            type="search"
            placeholder={tr("history.searchPlaceholder")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {query && (
            <button
              className="shrink-0 w-5 h-5 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors"
              onClick={() => setQuery("")}
              title="清除搜索"
            >
              <X size={13} />
            </button>
          )}
        </label>
      </div>

      {/* ── 列表 ── */}
      <div className="flex-1 min-h-0 overflow-y-auto">
        {!hasSessions ? (
          <div className="flex flex-col items-center justify-center gap-3 py-12 text-fg-faint">
            <MessageSquare size={32} className="opacity-20" />
            <div className="text-[13px]">{tr("history.empty")}</div>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 py-12 text-fg-faint">
            <Search size={32} className="opacity-20" />
            <div className="text-[13px]">{tr("history.noMatches")}</div>
            <button
              className="px-3 py-1 border border-border rounded text-fg-dim text-[11px] bg-transparent cursor-pointer hover:bg-bg-soft hover:text-fg transition-colors"
              onClick={() => setQuery("")}
            >
              清除搜索
            </button>
          </div>
        ) : (
          <div className="px-4 py-3.5 flex flex-col">
            {groups.map((g) => (
              <section className="mb-4" key={g.label}>
                <div className="flex items-center gap-2 text-fg-faint text-[10px] font-semibold uppercase tracking-wider px-2 pb-1.5">
                  <span className="w-1 h-1 rounded-full bg-fg-faint/30" />
                  {g.label}
                  <span className="text-fg-faint/40 font-mono font-normal normal-case tracking-normal">{g.items.length}</span>
                </div>
                {g.items.map((s) => (
                  <div
                    className={`group flex items-start gap-1 px-2 py-2.5 rounded-lg transition-colors duration-[0.12s] ${
                      s.current
                        ? "bg-sidebar-active border-l-[3px] border-l-accent"
                        : "border-l-[3px] border-l-transparent hover:bg-bg-soft"
                    }`}
                    key={s.path}
                  >
                    {editing === s.path ? (
                      <input
                        className="flex-1 bg-bg border border-accent rounded-md text-fg text-[13px] px-2 py-1 outline-none focus:shadow-[0_0_0_2px_var(--accent-soft)]"
                        autoFocus
                        value={draft}
                        onChange={(e) => setDraft(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") commitRename(s.path);
                          if (e.key === "Escape") setEditing(null);
                        }}
                        onBlur={() => { if (editing === s.path) commitRename(s.path); }}
                        placeholder={tr("history.namePlaceholder")}
                      />
                    ) : (
                      <button
                        className="flex-1 min-w-0 flex flex-col gap-1 bg-transparent border-0 text-left cursor-pointer"
                        onClick={() => onResume(s.path)}
                        title={s.path}
                      >
                        <div className={`text-[13px] leading-snug font-medium truncate ${s.current ? "text-accent" : "text-fg-dim"}`}>
                          {s.title || s.preview || tr("history.emptySession")}
                        </div>
                        <div className="flex items-center gap-1.5 text-fg-faint text-[11px]">
                          {s.current && (
                            <span className="bg-accent-soft text-accent text-[10px] px-1.5 py-px rounded font-medium">{tr("history.current")}</span>
                          )}
                          <span className="flex items-center gap-1">
                            <MessageSquare size={11} className="opacity-50" />
                            {tr(s.turns === 1 ? "history.turnOne" : "history.turnOther", { n: s.turns })}
                          </span>
                          <span className="text-fg-faint/40">·</span>
                          <span className="flex items-center gap-1">
                            <Clock size={11} className="opacity-50" />
                            {timeLabel(s.modTime)}
                          </span>
                        </div>
                      </button>
                    )}

                    {editing !== s.path && (
                      <div className="hidden group-hover:flex items-center gap-0.5 shrink-0">
                        {confirming === s.path ? (
                          <>
                            <button
                              className="w-7 h-7 flex items-center justify-center border-0 rounded-md bg-transparent text-err cursor-pointer hover:bg-err/10 transition-colors"
                              title={tr("history.confirmDelete")}
                              onClick={() => { onDelete(s.path); setConfirming(null); }}
                            ><Check size={14} /></button>
                            <button
                              className="w-7 h-7 flex items-center justify-center border-0 rounded-md bg-transparent text-fg-faint cursor-pointer hover:bg-bg-elev hover:text-fg transition-colors"
                              title={tr("common.cancel")}
                              onClick={() => setConfirming(null)}
                            ><X size={14} /></button>
                          </>
                        ) : (
                          <>
                            <button
                              className="w-7 h-7 flex items-center justify-center border-0 rounded-md bg-transparent text-fg-faint cursor-pointer hover:bg-bg-elev hover:text-fg transition-colors"
                              title={tr("history.rename")}
                              onClick={() => startRename(s)}
                            ><Pencil size={13} /></button>
                            {!s.current && (
                              <button
                                className="w-7 h-7 flex items-center justify-center border-0 rounded-md bg-transparent text-fg-faint cursor-pointer hover:bg-bg-elev hover:text-err transition-colors"
                                title={tr("common.delete")}
                                onClick={() => setConfirming(s.path)}
                              ><Trash2 size={13} /></button>
                            )}
                          </>
                        )}
                      </div>
                    )}
                  </div>
                ))}
              </section>
            ))}
          </div>
        )}
      </div>
    </ResizableDrawer>
  );
}

/** 日期分组标签 */
function dayLabel(ms: number): string {
  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((startOfDay(new Date()) - startOfDay(new Date(ms))) / 86_400_000);
  if (days <= 0) return t("history.today");
  if (days === 1) return t("history.yesterday");
  return new Date(ms).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function timeLabel(ms: number): string {
  return new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}
