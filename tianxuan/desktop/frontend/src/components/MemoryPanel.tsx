import { ChevronDown, ChevronRight, Pencil, Plus, RefreshCw, Search, Trash2 } from "lucide-react";
import { useCallback, useMemo, useRef, useState } from "react";
import { useT } from "../lib/i18n";
import type { MemoryArchive, MemoryFact, MemoryView } from "../lib/types";

type LinkInfo = { name: string; exists: boolean };

function displayTitle(fact: MemoryFact | MemoryArchive): string {
  return fact.title || fact.name.replaceAll("-", " ");
}

function uniqueLinks(body: string, names: Set<string>): LinkInfo[] {
  const links: LinkInfo[] = [];
  const seen = new Set<string>();
  const re = /\[\[([^\]]+)\]\]/g;
  let match: RegExpExecArray | null;
  while ((match = re.exec(body)) !== null) {
    const name = match[1].trim();
    if (!name || seen.has(name)) continue;
    seen.add(name);
    links.push({ name, exists: names.has(name) });
  }
  return links;
}

export function MemoryPanel({
  view,
  onClose,
  onRemember,
  onForget,
  onSaveDoc,
}: {
  view: MemoryView | null;
  onClose: () => void;
  onRemember: (scope: string, note: string) => Promise<void> | void;
  onForget: (name: string) => Promise<void> | void;
  onSaveDoc: (path: string, body: string) => Promise<void> | void;
}) {
  const t = useT();
  const [note, setNote] = useState("");
  const [scope, setScope] = useState("");
  const [editingPath, setEditingPath] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [highlight, setHighlight] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState("all");
  const [expandedFacts, setExpandedFacts] = useState<Set<string>>(new Set());
  const [confirmForget, setConfirmForget] = useState<string | null>(null);
  const [docsOpen, setDocsOpen] = useState(false);
  const [archivesOpen, setArchivesOpen] = useState(false);
  const factRefs = useRef<Record<string, HTMLElement | null>>({});

  const facts = view?.facts ?? [];
  const archives = view?.archives ?? [];
  const suggestions = view?.suggestions;
  const factNames = useMemo(() => new Set(facts.map((f) => f.name)), [facts]);
  const factTypes = useMemo(
    () => Array.from(new Set(facts.map((f) => f.type).filter(Boolean))).sort(),
    [facts],
  );
  const normalizedQuery = query.trim().toLowerCase();
  const filteredFacts = useMemo(
    () =>
      facts.filter((f) => {
        if (typeFilter !== "all" && f.type !== typeFilter) return false;
        if (!normalizedQuery) return true;
        return [displayTitle(f), f.name, f.description, f.type, f.body]
          .join(" ")
          .toLowerCase()
          .includes(normalizedQuery);
      }),
    [facts, normalizedQuery, typeFilter],
  );

  const scopes = view?.scopes ?? [];
  const activeScope = scope || scopes[0]?.scope || "";

  // Set initial scope
  if (!scope && scopes.length > 0) setScope(scopes[0].scope);

  const scrollToFact = (name: string) => {
    const el = factRefs.current[name];
    if (!el) return;
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    setHighlight(name);
    window.setTimeout(() => setHighlight((h) => (h === name ? null : h)), 1200);
  };

  const jumpTo = (name: string) => {
    const visible = filteredFacts.some((f) => f.name === name);
    if (!visible) {
      setQuery("");
      setTypeFilter("all");
      window.setTimeout(() => scrollToFact(name), 0);
      return;
    }
    scrollToFact(name);
  };

  const toggleFact = (name: string) => {
    setExpandedFacts((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const remember = useCallback(
    async (s: string, n: string) => {
      setBusy(true);
      try {
        await onRemember(s, n);
        setNote("");
      } finally {
        setBusy(false);
      }
    },
    [onRemember],
  );

  const forget = useCallback(
    async (name: string) => {
      setBusy(true);
      try {
        await onForget(name);
        setConfirmForget(null);
      } finally {
        setBusy(false);
      }
    },
    [onForget],
  );

  const submitNote = () => { if (note.trim()) void remember(activeScope, note.trim()); };

  const startEdit = (path: string, body: string) => {
    setEditingPath(path);
    setDraft(body);
  };

  const saveEdit = async () => {
    if (!editingPath) return;
    setBusy(true);
    try {
      await onSaveDoc(editingPath, draft);
      setEditingPath(null);
      setDraft("");
    } finally {
      setBusy(false);
    }
  };


  return (
    <div className="drawer-backdrop" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="drawer drawer--wide" data-state="open" style={{ maxWidth: 560 }}>
        {/* Header */}
        <div className="drawer__head">
          <div>
            <div className="drawer__title">{t("memory.title")}</div>
            {view && <div className="drawer__summary">{t("memory.summary", { facts: facts.length.toString(), docs: (view.docs.length).toString() })}</div>}
          </div>
          <button className="drawer__close" onClick={onClose} aria-label={t("common.close")}>×</button>
        </div>
        {/* ── Toolbar: search + type filter + refresh ── */}
        <div className="shrink-0 px-4 py-3 border-b border-border-soft space-y-2.5">
          <div className="flex items-center gap-2">
            <div className="flex items-center gap-1.5 flex-1 px-2.5 h-8 border border-border rounded-md bg-bg text-fg-faint focus-within:border-accent focus-within:shadow-[0_0_0_2px_var(--accent-soft)] transition-[border-color,box-shadow] duration-[var(--dur-fast)]">
              <Search size={14} />
              <input
                className="flex-1 min-w-0 border-0 outline-none bg-transparent text-fg text-[12.5px] placeholder:text-fg-faint"
                placeholder={t("memory.searchPlaceholder") ?? "搜索记忆…"}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
            <button
              className="shrink-0 w-8 h-8 flex items-center justify-center border border-border-soft rounded-md bg-transparent text-fg-faint cursor-pointer hover:text-fg hover:bg-bg-soft transition-[color,background] duration-[var(--dur-fast)]"
              onClick={onClose}
              title={t("memory.refresh") ?? "刷新"}
            >
              <RefreshCw size={14} />
            </button>
          </div>
          <div className="flex items-center gap-1.5 flex-wrap">
            <FilterChip active={typeFilter === "all"} label={t("memory.filterAll") ?? "全部"} onClick={() => setTypeFilter("all")} />
            {factTypes.map((ft) => (
              <FilterChip key={ft} active={typeFilter === ft} label={ft} onClick={() => setTypeFilter(ft)} />
            ))}
          </div>
        </div>

        {/* ── Body: facts + archives + suggestions ── */}
        <div className="flex-1 min-h-0 overflow-auto px-4 py-3">
          {/* Facts */}
          {filteredFacts.length === 0 && facts.length === 0 ? (
            <div className="py-10 text-center">
              <div className="text-fg-faint/50 text-[48px] mb-3">📝</div>
              <div className="text-fg-dim text-[13px] mb-1">{t("memory.empty") ?? "还没有记忆"}</div>
              <div className="text-fg-faint text-[11px]">{t("memory.emptyHint") ?? "使用下方的快速添加来创建第一条记忆"}</div>
            </div>
          ) : filteredFacts.length === 0 ? (
            <div className="py-10 text-center text-fg-faint text-[13px]">{t("memory.noResults") ?? "无匹配结果"}</div>
          ) : (
            <div className="flex flex-col gap-2">
              {filteredFacts.map((fact) => {
                const expanded = expandedFacts.has(fact.name);
                const links = uniqueLinks(fact.body, factNames);
                return (
                  <div
                    key={fact.name}
                    ref={(el) => { factRefs.current[fact.name] = el; }}
                    className={`mem-card border rounded-lg overflow-hidden transition-[border-color,box-shadow] duration-[var(--dur-fast)] ${
                      highlight === fact.name
                        ? "border-accent shadow-[0_0_0_2px_var(--accent-soft)]"
                        : "border-border-soft hover:border-fg-faint"
                    }`}
                  >
                    {/* Card header */}
                    <button
                      className="w-full flex items-start gap-2.5 px-3 py-2.5 bg-transparent border-0 text-left cursor-pointer hover:bg-bg-soft transition-colors duration-[var(--dur-fast)]"
                      onClick={() => toggleFact(fact.name)}
                    >
                      <span className="shrink-0 mt-0.5 text-fg-faint">
                        {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                      </span>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-fg text-[13px] font-medium truncate">{displayTitle(fact)}</span>
                          <span className="badge">{fact.type}</span>
                        </div>
                        {fact.description && (
                          <div className="text-fg-faint text-[11.5px] leading-relaxed mt-0.5 line-clamp-2">{fact.description}</div>
                        )}
                      </div>
                      <button
                        className="shrink-0 w-6 h-6 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint/60 cursor-pointer hover:text-err hover:bg-bg-soft transition-colors duration-[var(--dur-fast)]"
                        onClick={(e) => { e.stopPropagation(); setConfirmForget(fact.name); }}
                        title={t("memory.forget")}
                      >
                        <Trash2 size={13} />
                      </button>
                    </button>

                    {/* Expanded body */}
                    {expanded && (
                      <div className="px-3 pb-3 border-t border-border-soft">
                        <pre className="m-0 mt-2 bg-bg-soft border border-border-soft rounded-md p-3 text-fg-dim text-xs leading-relaxed whitespace-pre-wrap max-h-[360px] overflow-y-auto font-mono">
                          {fact.body}
                        </pre>
                        {links.length > 0 && (
                          <div className="mt-2 flex flex-wrap gap-1">
                            {links.map((l) => (
                              <button
                                key={l.name}
                                className={`px-2 py-0.5 rounded text-[10.5px] border cursor-pointer transition-colors duration-[var(--dur-fast)] ${
                                  l.exists
                                    ? "border-accent/30 bg-accent-soft text-accent hover:bg-accent/20"
                                    : "border-border-soft bg-transparent text-fg-faint line-through hover:bg-bg-soft"
                                }`}
                                onClick={(e) => { e.stopPropagation(); if (l.exists) jumpTo(l.name); }}
                                disabled={!l.exists}
                                title={l.exists ? `跳转到 ${l.name}` : `${l.name}（已删除）`}
                              >
                                {l.name}
                              </button>
                            ))}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}

          {/* Suggestions */}
          {suggestions && (suggestions.memory.length > 0 || suggestions.skills.length > 0) && (
            <div className="mt-4">
              <div className="text-fg text-[11px] font-semibold uppercase tracking-wider mb-2">{t("memory.suggestions") ?? "建议"}</div>
              <div className="flex flex-col gap-2">
                {suggestions.memory.map((s) => (
                  <div key={s.name} className="mem-suggestion border border-border-soft rounded-lg p-3 bg-bg-soft">
                    <div className="flex items-center gap-1.5 mb-1">
                      <span className="text-accent text-[11px] font-semibold uppercase tracking-wide">{t("memory.suggestionNew") ?? "建议新增"}</span>
                      <span className="badge">{s.type}</span>
                    </div>
                    <div className="text-fg text-[12.5px] font-medium">{s.title || s.name}</div>
                    <div className="text-fg-faint text-[11px] mt-0.5">{s.description}</div>
                    <div className="text-fg-faint/70 text-[10px] mt-1">{s.reason}</div>
                  </div>
                ))}
                {suggestions.skills.map((s) => (
                  <div key={s.name} className="mem-suggestion border border-border-soft rounded-lg p-3 bg-bg-soft">
                    <div className="flex items-center gap-1.5 mb-1">
                      <span className="text-info text-[11px] font-semibold uppercase tracking-wide">{t("memory.suggestionSkill") ?? "建议技能"}</span>
                    </div>
                    <div className="text-fg text-[12.5px] font-medium">{s.name}</div>
                    <div className="text-fg-faint text-[11px] mt-0.5">{s.description}</div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Archives */}
          {archives.length > 0 && (
            <div className="mt-4">
              <button
                className="flex items-center gap-1.5 text-fg-faint text-[11px] font-semibold uppercase tracking-wider bg-transparent border-0 cursor-pointer hover:text-fg transition-colors duration-[var(--dur-fast)]"
                onClick={() => setArchivesOpen((v) => !v)}
              >
                {archivesOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                {t("memory.archived") ?? "已归档"}
                <span className="text-fg-faint/60 font-normal">({archives.length})</span>
              </button>
              {archivesOpen && (
                <div className="mt-2 flex flex-col gap-1.5">
                  {archives.map((a) => (
                    <div
                      key={a.name}
                      className="border border-border-soft rounded-md px-3 py-2 bg-bg-soft/50 opacity-70 hover:opacity-100 transition-opacity duration-[var(--dur-fast)]"
                    >
                      <div className="flex items-center gap-2">
                        <span className="text-fg-dim text-[12px] font-medium">{displayTitle(a)}</span>
                        <span className="badge badge--muted">{a.type}</span>
                        {a.archivedAt && (
                          <span className="text-fg-faint text-[10px] ml-auto font-mono">{new Date(a.archivedAt).toLocaleDateString()}</span>
                        )}
                      </div>
                      {a.description && <div className="text-fg-faint text-[10.5px] mt-0.5">{a.description}</div>}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        {/* ── Footer: quick-add + doc files ── */}
        <div className="shrink-0 border-t border-border px-4 py-3 space-y-3">
          {/* Quick-add */}
          <div>
            <div className="flex items-center gap-2 mb-1.5">
              <span className="text-fg text-[12px] font-semibold">{t("memory.quickAdd")}</span>
              <span className="text-fg-faint text-[10px] font-mono">{scopes.find((s) => s.scope === activeScope)?.path}</span>
            </div>
            <div className="flex items-center gap-2">
              <select
                className="bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] px-2 py-1.5 outline-none focus:border-accent cursor-pointer"
                value={activeScope}
                onChange={(e) => setScope(e.target.value)}
              >
                {scopes.map((s) => (
                  <option key={s.scope} value={s.scope}>{s.scope}</option>
                ))}
              </select>
              <input
                className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
                placeholder={t("memory.notePlaceholder")}
                value={note}
                onChange={(e) => setNote(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") submitNote(); }}
              />
              <button
                className="px-3 py-1.5 border-0 rounded-md bg-accent text-accent-fg text-[12px] font-semibold cursor-pointer hover:brightness-110 active:scale-[0.97] transition-all duration-[var(--dur-fast)] disabled:opacity-40 disabled:cursor-default"
                onClick={submitNote}
                disabled={busy || !note.trim()}
              >
                <Plus size={13} className="inline mr-1" />
                {t("memory.remember")}
              </button>
            </div>
          </div>

          {/* Doc files */}
          {view && view.docs.length > 0 && (
            <div>
              <button
                className="flex items-center gap-1.5 text-fg text-[12px] font-semibold bg-transparent border-0 cursor-pointer py-0.5 hover:text-accent transition-colors duration-[var(--dur-fast)]"
                onClick={() => setDocsOpen((v) => !v)}
              >
                {docsOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                {t("memory.instructionFiles")}
                <span className="text-fg-faint text-[10px] font-normal ml-1">({view.docs.length})</span>
              </button>
              {docsOpen && (
                <div className="mt-2 flex flex-col gap-2">
                  {view.docs.map((d) => {
                    const editing = editingPath === d.path;
                    return (
                      <div className="mem-doc border border-border-soft rounded-lg overflow-hidden" key={d.path}>
                        <div className="flex items-center gap-2 px-2.5 py-1.5 bg-bg-soft/50">
                          <span className={`badge badge--${d.scope}`}>{d.scope}</span>
                          <span className="flex-1 text-fg-dim font-mono text-[10.5px] truncate" title={d.path}>
                            {d.path}
                          </span>
                          {!editing && (
                            <button
                              className="px-2 py-0.5 text-[10.5px] border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors duration-[var(--dur-fast)]"
                              onClick={() => startEdit(d.path, d.body)}
                            >
                              <Pencil size={11} className="inline mr-1" />
                              {t("common.edit") ?? "编辑"}
                            </button>
                          )}
                        </div>
                        {editing ? (
                          <div className="px-2.5 pb-2">
                            <textarea
                              className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] p-2 outline-none resize-y min-h-[100px] focus:border-accent font-mono"
                              value={draft}
                              onChange={(e) => setDraft(e.target.value)}
                              spellCheck={false}
                            />
                            <div className="flex justify-end gap-2 mt-1.5">
                              <button
                                className="px-2.5 py-1 text-[11px] border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors duration-[var(--dur-fast)]"
                                onClick={() => setEditingPath(null)}
                                disabled={busy}
                              >
                                {t("common.cancel") ?? "取消"}
                              </button>
                              <button
                                className="px-2.5 py-1 text-[11px] border-0 rounded bg-accent text-accent-fg font-semibold cursor-pointer hover:brightness-110 active:scale-[0.97] transition-all duration-[var(--dur-fast)] disabled:opacity-40"
                                onClick={() => void saveEdit()}
                                disabled={busy}
                              >
                                {t("common.save") ?? "保存"}
                              </button>
                            </div>
                          </div>
                        ) : (
                          <pre className="m-0 px-3 py-2 bg-bg text-fg-dim text-[11px] leading-relaxed whitespace-pre-wrap border-t border-border-soft max-h-[160px] overflow-y-auto font-mono">
                            {d.body}
                          </pre>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          {view?.storeDir && (
            <div className="text-fg-faint text-[9px] text-center font-mono" title={view.storeDir}>
              {view.storeDir}
            </div>
          )}
        </div>
      </div>

      {/* Confirm forget inline — shown at bottom of card */}
      {confirmForget && (
        <div className="ds-modal-overlay" onClick={() => setConfirmForget(null)}>
          <div
            className="ds-modal"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="text-fg text-[13px] mb-3">{t("memory.confirmForget")}</div>
            <div className="text-fg-faint text-[11px] mb-3 font-mono">{confirmForget}</div>
            <div className="flex justify-end gap-2">
              <button
                className="ds-button-pill"
                onClick={() => setConfirmForget(null)}
                disabled={busy}
              >
                {t("common.cancel") ?? "取消"}
              </button>
              <button
                className="px-3 py-1.5 text-[12px] border-0 rounded-full bg-err text-white font-semibold cursor-pointer hover:brightness-110 active:scale-[0.97] transition-all duration-[var(--dur-fast)] disabled:opacity-40"
                onClick={() => void forget(confirmForget)}
                disabled={busy}
              >
                {t("memory.forget")}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/** 类型筛选小标签 */
function FilterChip({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      className={`ds-chip ${active ? "ds-chip--accent" : "ds-chip--muted"}`}
      onClick={onClick}
      type="button"
    >
      {label}
    </button>
  );
}
