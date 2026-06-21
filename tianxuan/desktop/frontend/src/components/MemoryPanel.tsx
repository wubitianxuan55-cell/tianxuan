import { ChevronDown, ChevronRight, Search, Trash2 } from "lucide-react";
import { useMemo, useRef, useState, type ReactNode } from "react";
import { useT } from "../lib/i18n";
import type { MemoryFact, MemoryView } from "../lib/types";
import { CloseButton } from "./CloseButton";

type LinkInfo = { name: string; exists: boolean };

function displayTitle(fact: MemoryFact): string {
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
  const [expanded, setExpanded] = useState<string | null>(null);
  const [confirmForget, setConfirmForget] = useState<string | null>(null);
  const [docsOpen, setDocsOpen] = useState(false);
  const factRefs = useRef<Record<string, HTMLElement | null>>({});

  const facts = view?.facts ?? [];
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
          .join(" ").toLowerCase().includes(normalizedQuery);
      }),
    [facts, normalizedQuery, typeFilter],
  );

  const scrollToFact = (name: string) => {
    const el = factRefs.current[name];
    if (!el) return;
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    setHighlight(name);
    window.setTimeout(() => setHighlight((h) => (h === name ? null : h)), 1200);
  };

  const jumpTo = (name: string) => {
    if (!factNames.has(name)) return;
    const visible = filteredFacts.some((f) => f.name === name);
    setExpanded(name);
    setConfirmForget(null);
    if (!visible) { setQuery(""); setTypeFilter("all"); window.setTimeout(() => scrollToFact(name), 0); return; }
    scrollToFact(name);
  };

  const renderWithLinks = (text: string): ReactNode[] => {
    const out: ReactNode[] = [];
    const re = /\[\[([^\]]+)\]\]/g;
    let last = 0, k = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(text)) !== null) {
      if (m.index > last) out.push(text.slice(last, m.index));
      const target = m[1].trim();
      out.push(
        factNames.has(target) ? (
          <button key={k++} type="button" className="inline-flex items-center gap-1 px-1.5 py-px border border-border-soft rounded text-fg-faint text-[11px] cursor-pointer hover:text-fg hover:border-fg-faint transition-colors" onClick={() => jumpTo(target)}>{target}</button>
        ) : (
          <span key={k++} className="text-err line-through underline decoration-dotted cursor-help" title={t("memory.deadLink", { name: target })}>{target}</span>
        ),
      );
      last = re.lastIndex;
    }
    if (last < text.length) out.push(text.slice(last));
    return out;
  };

  const forgetFact = async (name: string) => {
    if (busy) return;
    setBusy(true); try { await onForget(name); if (expanded === name) setExpanded(null); setConfirmForget(null); } finally { setBusy(false); }
  };

  const scopes = view?.scopes ?? [];
  const activeScope = scope || scopes.find((s) => s.scope === "project")?.scope || scopes[0]?.scope || "project";

  const submitNote = async () => {
    const trimmed = note.trim();
    if (!trimmed || busy) return;
    setBusy(true); try { await onRemember(activeScope, trimmed); setNote(""); } finally { setBusy(false); }
  };

  const startEdit = (path: string, body: string) => { setEditingPath(path); setDraft(body); };
  const saveEdit = async () => {
    if (editingPath === null || busy) return;
    setBusy(true); try { await onSaveDoc(editingPath, draft); setEditingPath(null); } finally { setBusy(false); }
  };

  // ─── backdrop click → close ──────────────────────────────────────
  const handleBackdrop = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) onClose();
  };

  return (
    <div className="fixed inset-0 z-50">
      {/* 不可见点击-关闭层 */}
      <div className="absolute inset-0" onClick={handleBackdrop} />
      {/* 仅视觉遮罩 —— 不拦截滚轮 */}
      <div className="absolute inset-0 bg-bg/60 pointer-events-none animate-[fadeIn_.15s_ease-out]" />
      {/* 居中布局 */}
      <div className="absolute inset-0 flex items-center justify-center p-6 pointer-events-none">
      <div className="flex flex-col w-full max-w-2xl max-h-[88vh] bg-bg-elev border border-border rounded-xl shadow-[0_16px_48px_rgba(0,0,0,0.35)] animate-[scaleIn_.2s_ease-out] overflow-hidden pointer-events-auto">
        {/* ── Header ── */}
        <header className="flex items-center justify-between shrink-0 px-5 py-3.5 border-b border-border">
          <div className="flex items-center gap-3 min-w-0">
            <span className="text-[15px] font-semibold text-fg">{t("memory.title")}</span>
            {view?.available && (
              <span className="text-fg-faint text-[11px] bg-bg-soft px-2 py-0.5 rounded-full font-mono">
                {facts.length} 记忆 · {view.docs.length} 文档
              </span>
            )}
          </div>
          <CloseButton onClick={onClose} />
        </header>

        {!view?.available ? (
          <div className="flex-1 flex items-center justify-center py-12 text-fg-faint text-sm">{t("memory.unavailable")}</div>
        ) : (
          <>
            {/* ── 搜索 + 筛选（粘性）── */}
            <div className="shrink-0 px-5 py-3 border-b border-border-soft bg-bg-soft/30">
              <div className="flex items-center gap-2">
                <label className="flex items-center gap-1.5 flex-1 px-2.5 h-8 border border-border rounded-md bg-bg text-fg-faint focus-within:border-accent focus-within:shadow-[0_0_0_2px_var(--accent-soft)] transition-[border-color,box-shadow] duration-[0.12s]">
                  <Search size={14} />
                  <input
                    className="flex-1 border-0 outline-none bg-transparent text-fg text-[13px] placeholder:text-fg-faint"
                    value={query} onChange={(e) => setQuery(e.target.value)}
                    placeholder={t("memory.searchPlaceholder")}
                  />
                </label>
                <div className="flex gap-1" role="tablist">
                  <FilterChip active={typeFilter === "all"} label={t("memory.allTypes")} onClick={() => setTypeFilter("all")} />
                  {factTypes.map((type) => (
                    <FilterChip key={type} active={typeFilter === type} label={type} onClick={() => setTypeFilter(type)} />
                  ))}
                </div>
              </div>
            </div>

            {/* ── 事实列表（可滚动）── */}
            <div className="flex-1 min-h-0 overflow-y-auto px-5 py-3">
              {facts.length === 0 ? (
                <div className="py-8 text-fg-faint text-xs text-center">
                  <div className="text-[13px] mb-1">{t("memory.noFacts")}</div>
                  <div className="text-[11px] opacity-60">{t("memory.fallibleNote")}</div>
                </div>
              ) : filteredFacts.length === 0 ? (
                <div className="py-8 text-fg-faint text-xs text-center">
                  {t("memory.noMatches")}
                  <button className="mt-2 px-3 py-1 border border-border rounded text-fg-dim text-[11px] bg-transparent cursor-pointer hover:bg-bg-soft hover:text-fg transition-colors" onClick={() => { setQuery(""); setTypeFilter("all"); }} type="button">
                    {t("memory.clearFilters")}
                  </button>
                </div>
              ) : (
                <div className="flex flex-col gap-2.5">
                  {filteredFacts.map((f) => {
                    const isOpen = expanded === f.name;
                    const links = uniqueLinks(f.body, factNames);
                    const missing = links.filter((l) => !l.exists);
                    return (
                      <article
                        className={`border rounded-lg overflow-hidden transition-[border-color,box-shadow] duration-200 ${
                          highlight === f.name ? "ring-2 ring-accent/30 border-accent/40" : "border-border-soft"
                        } ${isOpen ? "border-l-[3px] border-l-accent" : ""}`}
                        key={f.name}
                        ref={(el) => { factRefs.current[f.name] = el; }}
                      >
                        <button
                          className="flex items-start gap-2 w-full px-3 py-2.5 border-0 bg-transparent text-left cursor-pointer hover:bg-bg-soft transition-colors"
                          onClick={() => { setExpanded(isOpen ? null : f.name); setConfirmForget(null); }}
                          type="button"
                        >
                          {isOpen ? <ChevronDown size={15} className="shrink-0 mt-0.5 text-fg-faint" /> : <ChevronRight size={15} className="shrink-0 mt-0.5 text-fg-faint" />}
                          <span className="flex-1 min-w-0 flex flex-col gap-0.5">
                            <span className="flex items-center gap-2">
                              <span className="text-fg text-[13px] font-semibold leading-tight">{displayTitle(f)}</span>
                              <span className="text-fg-faint/50 font-mono text-[10px]">{f.type}</span>
                            </span>
                            <span className="text-fg-faint font-mono text-[10px]">{f.name}</span>
                            <span className="text-fg-dim text-[11.5px] leading-snug line-clamp-2">{f.description}</span>
                          </span>
                        </button>

                        {/* 内联链接预览 */}
                        {links.length > 0 && (
                          <div className="flex flex-wrap gap-1 px-3 pb-1.5">
                            {links.map((link) =>
                              link.exists ? (
                                <button className="inline-flex items-center px-2 py-0.5 border border-accent/20 rounded text-accent text-[11px] font-mono bg-transparent cursor-pointer hover:bg-accent-soft transition-colors" key={link.name} onClick={() => jumpTo(link.name)} type="button">[[{link.name}]]</button>
                              ) : (
                                <span className="inline-flex items-center px-2 py-0.5 border border-border-soft rounded text-fg-faint text-[11px] font-mono opacity-50" key={link.name}>[[{link.name}]]</span>
                              ),
                            )}
                          </div>
                        )}

                        {/* 展开详情 */}
                        {isOpen && (
                          <div className="border-t border-border-soft px-3 py-2.5">
                            {f.body ? (
                              <div className="text-fg-dim text-xs leading-relaxed pb-2">{renderWithLinks(f.body)}</div>
                            ) : (
                              <div className="py-3 text-fg-faint text-xs text-center">{t("memory.noBody")}</div>
                            )}
                            {missing.length > 0 && (
                              <div className="mb-2 text-fg-faint text-[11px]">{t("memory.missingLinks", { n: missing.length })}</div>
                            )}
                            <div className="flex items-center justify-between">
                              <span className="text-fg-faint text-[10px]">{t("memory.appliesNow")}</span>
                              {confirmForget === f.name ? (
                                <div className="flex items-center gap-1.5">
                                  <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" onClick={() => setConfirmForget(null)} disabled={busy} type="button">{t("common.cancel")}</button>
                                  <button className="px-2.5 py-1 text-xs border rounded cursor-pointer transition-colors text-err border-err/30 bg-err/10 hover:bg-err/20" onClick={() => void forgetFact(f.name)} disabled={busy} type="button">{t("memory.confirmForget")}</button>
                                </div>
                              ) : (
                                <button className="px-2.5 py-1 text-xs inline-flex items-center gap-[5px] border border-transparent rounded bg-transparent text-fg-dim cursor-pointer hover:text-err hover:border-err/30 hover:bg-err/10 transition-colors" onClick={() => setConfirmForget(f.name)} disabled={busy} type="button"><Trash2 size={13} />{t("memory.forget")}</button>
                              )}
                            </div>
                          </div>
                        )}
                      </article>
                    );
                  })}
                </div>
              )}
              {view.storeDir && (
                <div className="mt-3 text-fg-faint text-[10px] text-center" title={view.storeDir}>
                  {t("memory.storedUnder", { dir: view.storeDir })}
                </div>
              )}
            </div>

            {/* ── 底部操作区 ── */}
            <div className="shrink-0 border-t border-border px-5 py-3 space-y-3">
              {/* Quick-add */}
              <div>
                <div className="flex items-center gap-2 mb-1.5">
                  <span className="text-fg text-[13px] font-semibold">{t("memory.quickAdd")}</span>
                  <span className="text-fg-faint text-[10px]">
                    {scopes.find((s) => s.scope === activeScope)?.path}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <select
                    className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2 py-1.5 outline-none focus:border-accent cursor-pointer"
                    value={activeScope} onChange={(e) => setScope(e.target.value)}
                  >
                    {scopes.map((s) => (<option key={s.scope} value={s.scope}>{s.scope}</option>))}
                  </select>
                  <input
                    className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
                    placeholder={t("memory.notePlaceholder")}
                    value={note} onChange={(e) => setNote(e.target.value)}
                    onKeyDown={(e) => { if (e.key === "Enter") void submitNote(); }}
                  />
                  <button className="btn--primary" onClick={() => void submitNote()} disabled={busy || !note.trim()}>
                    {t("memory.remember")}
                  </button>
                </div>
              </div>

              {/* Doc files — 可折叠 */}
              {view.docs.length > 0 && (
                <div>
                  <button
                    className="flex items-center gap-1.5 text-fg text-[13px] font-semibold bg-transparent border-0 cursor-pointer py-0.5 hover:text-accent transition-colors"
                    onClick={() => setDocsOpen((v) => !v)}
                  >
                    {docsOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                    {t("memory.instructionFiles")}
                    <span className="text-fg-faint text-[11px] font-normal ml-1">({view.docs.length})</span>
                  </button>
                  {docsOpen && (
                    <div className="mt-2 flex flex-col gap-2">
                      {view.docs.map((d) => {
                        const editing = editingPath === d.path;
                        return (
                          <div className="border border-border-soft rounded-lg overflow-hidden" key={d.path}>
                            <div className="flex items-center gap-2 px-2.5 py-1.5">
                              <span className={`badge badge--${d.scope}`}>{d.scope}</span>
                              <span className="flex-1 text-fg-dim font-mono text-[11px] truncate" title={d.path}>{d.path}</span>
                              {!editing && (
                                <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" onClick={() => startEdit(d.path, d.body)}>{t("common.edit")}</button>
                              )}
                            </div>
                            {editing ? (
                              <div className="px-2.5 pb-2">
                                <textarea
                                  className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] p-2 outline-none resize-y min-h-[120px] focus:border-accent"
                                  value={draft} onChange={(e) => setDraft(e.target.value)} spellCheck={false}
                                />
                                <div className="flex justify-end gap-2 mt-1.5">
                                  <button className="px-2.5 py-1 text-xs border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors" onClick={() => setEditingPath(null)} disabled={busy}>{t("common.cancel")}</button>
                                  <button className="btn--primary" onClick={() => void saveEdit()} disabled={busy}>{t("common.save")}</button>
                                </div>
                              </div>
                            ) : (
                              <pre className="m-0 px-2.5 py-2 bg-bg text-fg-dim text-xs leading-relaxed whitespace-pre-wrap border-t border-border-soft max-h-[200px] overflow-y-auto">{d.body}</pre>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              )}
            </div>
          </>
        )}
      </div>
      </div>
    </div>
  );
}

/** 类型筛选小标签 */
function FilterChip({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      className={`px-2.5 py-1 border rounded-md text-[11px] bg-transparent cursor-pointer transition-[color,background,border] duration-[0.12s] hover:bg-bg-soft hover:text-fg ${active ? "bg-accent-soft text-accent border-accent/30" : "border-border-soft text-fg-dim"}`}
      onClick={onClick}
      type="button"
    >
      {label}
    </button>
  );
}
