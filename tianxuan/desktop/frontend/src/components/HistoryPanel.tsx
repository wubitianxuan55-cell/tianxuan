import { useMemo, useState } from "react";
import { Pencil, Search, Trash2, Check, X } from "lucide-react";
import { t, useT } from "../lib/i18n";
import type { SessionMeta } from "../lib/types";
import { ResizableDrawer } from "./ResizableDrawer";

// HistoryPanel is the desktop session switcher: a right-side drawer listing saved
// sessions newest-first, grouped by day. Each row resumes on click, and carries
// rename (a custom display name) and delete actions on hover — the desktop
// analogue of managing conversations in Claude Code. The active session can't be
// deleted (auto-save would just recreate it).
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
    onRename(path, draft.trim());
    setEditing(null);
  };

  // Sessions arrive newest-first; bucket consecutive ones under a day heading
  // (Today / Yesterday / a date) while preserving that order.
  const groups: { label: string; items: SessionMeta[] }[] = [];
  for (const s of filtered) {
    const label = dayLabel(s.modTime);
    const last = groups[groups.length - 1];
    if (last && last.label === label) last.items.push(s);
    else groups.push({ label, items: [s] });
  }

  return (
    <ResizableDrawer onClose={onClose}>
        <header className="flex items-center justify-between px-4 py-3.5 bg-bg-elev border-b border-border">
          <div className="text-[15px] font-semibold text-fg">{tr("history.title")}</div>
          <button className="inline-flex items-center gap-[5px] h-[26px] px-[11px] border border-border bg-bg-soft text-fg-dim text-xs rounded-[7px] cursor-pointer transition-[color,border-color,background] duration-[0.12s] hover:text-fg hover:border-fg-faint disabled:opacity-40 disabled:cursor-default disabled:hover:text-fg-dim disabled:hover:border-border no-drag" onClick={onClose} title={tr("common.close")}>
            ✕
          </button>
        </header>

        <div className="flex items-center gap-2 mx-3 mt-2 px-3 h-9 border border-border rounded-lg bg-bg-soft text-fg-faint focus-within:border-accent">
          <Search size={14} className="text-fg-faint shrink-0" />
          <input
            className="flex-1 border-0 outline-none bg-transparent text-fg text-[13px] placeholder:text-fg-faint"
            type="search"
            placeholder={tr("history.searchPlaceholder")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>

        <div className="overflow-y-auto px-4 py-3.5 flex flex-col gap-[22px]">
          {sessions.length === 0 ? (
            <div className="py-5 text-fg-faint text-xs text-center">{tr("history.empty")}</div>
          ) : filtered.length === 0 ? (
            <div className="py-5 text-fg-faint text-xs text-center">{tr("history.noMatches")}</div>
          ) : (
            groups.map((g) => (
              <section className="mb-3" key={g.label}>
                <div className="text-fg-faint font-mono text-[11px] uppercase tracking-wider px-1 pb-1">{g.label}</div>
                {g.items.map((s) => (
                  <div className={`group flex items-start gap-1 px-2 py-2 rounded-lg hover:bg-bg-soft ${s.current ? "bg-sidebar-active" : ""}`} key={s.path}>
                    {editing === s.path ? (
                      <input
                        className="flex-1 bg-bg border border-accent rounded-md text-fg text-[13px] px-2 py-1 outline-none"
                        autoFocus
                        value={draft}
                        onChange={(e) => setDraft(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") commitRename(s.path);
                          if (e.key === "Escape") setEditing(null);
                        }}
                        onBlur={() => commitRename(s.path)}
                        placeholder={tr("history.namePlaceholder")}
                      />
                    ) : (
                      <button className="flex-1 min-w-0 flex flex-col gap-0.5 bg-transparent border-0 text-left cursor-pointer" onClick={() => onResume(s.path)} title={s.path}>
                        <div className="text-fg-dim text-[13px] leading-snug font-medium truncate">{s.title || s.preview || tr("history.emptySession")}</div>
                        <div className="flex items-center gap-1.5 text-fg-faint text-[11px]">
                          {s.current && <span className="bg-accent-soft text-accent text-[10px] px-1.5 py-px rounded font-medium">{tr("history.current")}</span>}
                          <span>{tr(s.turns === 1 ? "history.turnOne" : "history.turnOther", { n: s.turns })}</span>
                          <span>·</span>
                          <span>{timeLabel(s.modTime)}</span>
                        </div>
                      </button>
                    )}

                    {editing !== s.path && (
                      <div className="hidden group-hover:flex items-center gap-1 shrink-0">
                        {confirming === s.path ? (
                          <>
                            <button className="w-7 h-7 flex items-center justify-center border-0 rounded-md bg-transparent text-err cursor-pointer hover:bg-bg-elev" title={tr("history.confirmDelete")} onClick={() => { onDelete(s.path); setConfirming(null); }}><Check size={14} /></button>
                            <button className="w-7 h-7 flex items-center justify-center border-0 rounded-md bg-transparent text-fg-faint cursor-pointer hover:bg-bg-elev hover:text-fg" title={tr("common.cancel")} onClick={() => setConfirming(null)}><X size={14} /></button>
                          </>
                        ) : (
                          <>
                            <button className="w-7 h-7 flex items-center justify-center border-0 rounded-md bg-transparent text-fg-faint cursor-pointer hover:bg-bg-elev hover:text-fg" title={tr("history.rename")} onClick={() => startRename(s)}><Pencil size={13} /></button>
                            {!s.current && (
                              <button className="w-7 h-7 flex items-center justify-center border-0 rounded-md bg-transparent text-fg-faint cursor-pointer hover:bg-bg-elev hover:text-err" title={tr("common.delete")} onClick={() => setConfirming(s.path)}><Trash2 size={13} /></button>
                            )}
                          </>
                        )}
                      </div>
                    )}
                  </div>
                ))}
              </section>
            ))
          )}
        </div>
    </ResizableDrawer>
  );
}

// dayLabel buckets a timestamp into "Today", "Yesterday", or a locale date. It's
// module-level (not a component), so it uses the non-reactive translator; the
// panel re-renders on a locale switch via its parent, picking up the new strings.
function dayLabel(ms: number): string {
  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((startOfDay(new Date()) - startOfDay(new Date(ms))) / 86_400_000);
  if (days <= 0) return t("history.today");
  if (days === 1) return t("history.yesterday");
  return new Date(ms).toLocaleDateString();
}

function timeLabel(ms: number): string {
  return new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}
