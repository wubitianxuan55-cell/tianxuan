import { Check, ChevronDown, ChevronRight, FileText, Pencil, Plus, RefreshCw, Search, Sparkles, Trash2, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { MemoryArchive, MemoryFact, MemorySuggestion, MemorySuggestionsView, MemoryView, SkillSuggestion, TabMeta } from "../lib/types";
import { factTypeLabel } from "../lib/factTypeLabel";
import { ResizableDrawer } from "./ResizableDrawer";
import { Tooltip } from "./Tooltip";

// ── helpers ───────────────────────────────────────────────────────────

type LinkInfo = { name: string; exists: boolean };

function displayTitle(fact: MemoryFact): string {
  return fact.title || fact.name.replaceAll("-", " ");
}

function memoryMatches(fact: MemoryFact, normalizedQuery: string, typeFilter: string): boolean {
  if (typeFilter !== "all" && fact.type !== typeFilter) return false;
  if (!normalizedQuery) return true;
  return [displayTitle(fact), fact.name, fact.description, fact.type, fact.body]
    .join(" ")
    .toLowerCase()
    .includes(normalizedQuery);
}

function archiveKey(f: MemoryArchive): string {
  return `${f.path || f.name}:${f.archivedAt || ""}`;
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

function scopeLabel(scope: string, t: ReturnType<typeof useT>): string {
  return { project: t("memory.scopeProject"), user: t("memory.scopeUser"), local: t("memory.scopeLocal") }[scope] || scope;
}

function autoSuggestionsKey(): string { return "tianxuan-mem-auto-suggestions"; }
function readAutoPref(): boolean {
  try { return localStorage.getItem(autoSuggestionsKey()) !== "false"; } catch { return true; }
}
function writeAutoPref(v: boolean) {
  try { localStorage.setItem(autoSuggestionsKey(), String(v)); } catch { /* noop */ }
}

// ── MemoryPanel ────────────────────────────────────────────────────────

export function MemoryPanel(p: {
  onClose: () => void;
  onRemember: (scope: string, note: string) => Promise<void> | void;
  onForget: (name: string) => Promise<void> | void;
  onSaveDoc: (path: string, body: string) => Promise<void> | void;
  onSaveFact: (name: string, body: string) => Promise<void> | void;
  onChangeType: (name: string, newType: string) => Promise<void> | void;
  onAcceptMemorySuggestion: (c: MemorySuggestion) => Promise<void> | void;
  onAcceptSkillSuggestion: (c: SkillSuggestion) => Promise<void> | void;
  onRefreshSuggestions: () => Promise<MemorySuggestionsView | null>;
}) {
  const t = useT();
  const { onClose, onRemember, onForget, onSaveDoc, onSaveFact, onChangeType: _changeType,
    onAcceptMemorySuggestion, onAcceptSkillSuggestion, onRefreshSuggestions } = p;
  void _changeType; // accepted but not yet wired in new UI

  // ── state ──
  const [tabs, setTabs] = useState<TabMeta[]>([]);
  const [selTabId, setSelTabId] = useState<string | null>(null);
  const [view, setView] = useState<MemoryView | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState("");
  const [scope, setScope] = useState("");
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState("all");
  const [expanded, setExpanded] = useState<string | null>(null);
  const [expandedArchive, setExpandedArchive] = useState<string | null>(null);
  const [confirmForget, setConfirmForget] = useState<string | null>(null);
  const [editingPath, setEditingPath] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [highlight, setHighlight] = useState<string | null>(null);
  const [tab, setTab] = useState<"saved" | "archived" | "docs" | "suggestions">("saved");
  // suggestions
  const [suggestions, setSuggestions] = useState<MemorySuggestionsView | null>(null);
  const [suggestionBusy, setSuggestionBusy] = useState(false);
  const [acceptedSuggestions, setAcceptedSuggestions] = useState<Set<string>>(new Set());
  const [autoSuggestions, setAutoSuggestions] = useState(readAutoPref);
  const autoReq = useRef(false);
  const factRefs = useRef<Record<string, HTMLElement | null>>({});
  const highlightTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── load tabs ──
  useEffect(() => {
    app.TabMeta().then((tl) => {
      setTabs(tl);
      if (!selTabId && tl.length > 0) setSelTabId(tl[0].id);
    }).catch(() => {});
  }, []);

  const uniqueTabs = useMemo(() => {
    const byWs = new Map<string, TabMeta>();
    for (const tb of tabs) {
      const key = tb.workspaceRoot || `${tb.scope}:global`;
      if (!byWs.has(key)) byWs.set(key, tb);
    }
    return [...byWs.values()];
  }, [tabs]);

  const effectiveTabId = useMemo(() => {
    if (uniqueTabs.some((tb) => tb.id === selTabId)) return selTabId;
    return uniqueTabs[0]?.id ?? null;
  }, [selTabId, uniqueTabs]);

  // ── data loading ──
  const reload = useCallback(async () => {
    const tabId = effectiveTabId;
    setView((prev) => prev && tabId ? { ...prev, facts: [], archives: [], docs: [] } : prev);
    try {
      const v = tabId ? await app.MemoryForTab(tabId) : await app.Memory();
      setView(v);
    } catch { setView(null); }
  }, [effectiveTabId]);

  useEffect(() => { void reload(); }, [reload]);

  // ── derived ──
  const facts = view?.facts ?? [];
  const archives = view?.archives ?? [];
  const docs = view?.docs ?? [];
  const scopes = view?.scopes ?? [];
  const activeScope = scope || scopes.find((s) => s.scope === "project")?.scope || scopes[0]?.scope || "project";
  const factNames = useMemo(() => new Set(facts.map((f) => f.name)), [facts]);
  const factTypes = useMemo(() => Array.from(new Set([...facts, ...archives].map((f) => f.type).filter(Boolean))).sort(), [facts, archives]);
  const normalizedQuery = query.trim().toLowerCase();
  const filteredFacts = useMemo(() => facts.filter((f) => memoryMatches(f, normalizedQuery, typeFilter)), [facts, normalizedQuery, typeFilter]);
  const filteredArchives = useMemo(() => archives.filter((f) => {
    if (typeFilter !== "all" && f.type !== typeFilter) return false;
    if (!normalizedQuery) return true;
    return memoryMatches(f, normalizedQuery, "all") || [f.path, f.archivedAt].join(" ").toLowerCase().includes(normalizedQuery);
  }), [archives, normalizedQuery, typeFilter]);
  const filteredDocs = useMemo(() => docs.filter((d) => {
    if (!normalizedQuery) return true;
    return `${d.path} ${d.body}`.toLowerCase().includes(normalizedQuery);
  }), [docs, normalizedQuery]);

  // ── actions ──
  const scrollToFact = useCallback((name: string) => {
    const el = factRefs.current[name];
    if (!el) return;
    el.scrollIntoView({ block: "center", behavior: "auto" });
    setHighlight(name);
    if (highlightTimer.current) clearTimeout(highlightTimer.current);
    highlightTimer.current = setTimeout(() => setHighlight((h) => (h === name ? null : h)), 1200);
  }, []);

  const jumpTo = useCallback((name: string) => {
    if (!factNames.has(name)) return;
    const visible = filteredFacts.some((f) => f.name === name);
    setExpanded(name);
    setConfirmForget(null);
    if (!visible) { setQuery(""); setTypeFilter("all"); setTimeout(() => scrollToFact(name), 0); return; }
    scrollToFact(name);
  }, [factNames, filteredFacts, scrollToFact]);

  const renderWithLinks = useCallback((text: string): ReactNode[] => {
    const out: ReactNode[] = [];
    const re = /\[\[([^\]]+)\]\]/g;
    let last = 0, k = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(text)) !== null) {
      if (m.index > last) out.push(text.slice(last, m.index));
      const target = m[1].trim();
      if (factNames.has(target)) {
        out.push(<button key={k++} type="button" className="mem-link" onClick={() => jumpTo(target)}>{target}</button>);
      } else {
        out.push(<Tooltip key={k++} label={t("memory.deadLink", { name: target })}><span className="mem-link mem-link--dead">{target}</span></Tooltip>);
      }
      last = re.lastIndex;
    }
    if (last < text.length) out.push(text.slice(last));
    return out;
  }, [factNames, jumpTo, t]);

  const submitNote = useCallback(async () => {
    if (!note.trim() || busy) return;
    setBusy(true);
    try { await onRemember(activeScope, note.trim()); setNote(""); await reload(); }
    catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [note, busy, activeScope, onRemember, reload]);

  const forgetFact = useCallback(async (name: string) => {
    if (busy) return;
    setBusy(true);
    try {
      if (effectiveTabId) await app.ForgetForTab(effectiveTabId, name);
      else await onForget(name);
      await reload();
      if (expanded === name) setExpanded(null);
      setConfirmForget(null);
    } catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [busy, expanded, reload, effectiveTabId, onForget]);

  const refreshSuggestions = useCallback(async () => {
    if (suggestionBusy) return;
    setSuggestionBusy(true);
    setError(null);
    try {
      const next = effectiveTabId
        ? await app.MemorySuggestionsForTab(effectiveTabId)
        : await onRefreshSuggestions();
      setSuggestions(next);
      setAcceptedSuggestions(new Set());
    } catch (e) { setError(String(e)); }
    finally { setSuggestionBusy(false); }
  }, [effectiveTabId, onRefreshSuggestions, suggestionBusy]);

  const startEdit = useCallback((path: string, body: string) => { setEditingPath(path); setDraft(body); }, []);
  const saveEdit = useCallback(async () => {
    if (!editingPath || busy) return;
    setBusy(true);
    try { await onSaveDoc(editingPath, draft); setEditingPath(null); await reload(); }
    catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [editingPath, draft, busy, onSaveDoc, reload]);

  // auto suggestions
  useEffect(() => {
    if (tab !== "suggestions" || !autoSuggestions || suggestions || suggestionBusy || autoReq.current) return;
    autoReq.current = true;
    void refreshSuggestions();
  }, [autoSuggestions, refreshSuggestions, suggestionBusy, suggestions, tab]);

  const toggleAuto = useCallback((v: boolean) => { autoReq.current = false; setAutoSuggestions(v); writeAutoPref(v); }, []);

  // ── keyboard shortcuts ──
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "/" && document.activeElement === document.body && tab === "saved") {
        e.preventDefault(); (document.querySelector(".mem-filter") as HTMLInputElement)?.focus();
      }
      if (e.ctrlKey && e.key === "n") {
        e.preventDefault(); (document.querySelector(".mem-input") as HTMLInputElement)?.focus();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [tab]);

  useEffect(() => () => { if (highlightTimer.current) clearTimeout(highlightTimer.current); }, []);

  // ── workspace selector ──
  const selectedWs = uniqueTabs.find((tb) => tb.id === effectiveTabId);
  const wsSelector = uniqueTabs.length > 1 ? (
    <select
      className="mem-select text-sm"
      value={effectiveTabId ?? ""}
      onChange={(e) => setSelTabId(e.target.value)}
    >
      {uniqueTabs.map((tb) => (
        <option key={tb.id} value={tb.id}>
          {tb.workspaceName || tb.label || tb.title || tb.scope || tb.id}
        </option>
      ))}
    </select>
  ) : (
    <span className="text-fg-dim text-sm font-medium truncate max-w-[200px]">
      {selectedWs?.workspaceName || selectedWs?.label || ""}
    </span>
  );

  const scopePath = scopes.find((s) => s.scope === activeScope)?.path;

  // ── render ──
  return (
    <ResizableDrawer onClose={onClose} wide>
      {/* header */}
      <div className="drawer__head">
        <div>
          <div className="drawer__title">{t("memory.title")}</div>
          <div className="mt-1">{wsSelector}</div>
        </div>
        <button className="drawer__close" onClick={onClose} aria-label={t("common.close")}><X size={18} /></button>
      </div>
      {/* error banner */}
      {error && (
        <div className="mx-4 mt-2 px-3 py-2 rounded-lg bg-err/10 border border-err/20 text-err text-xs flex items-center justify-between">
          <span>{error}</span>
          <button className="bg-transparent border-0 text-err cursor-pointer hover:bg-err/10 rounded p-0.5 transition-colors duration-150 focus-visible:ring-1 focus-visible:ring-err/30 focus-visible:outline-none" onClick={() => setError(null)}><X size={13} /></button>
        </div>
      )}

      {/* quick-add */}
      <section className="mem-section">
        <div className="mem-section__row">
          <div className="mem-section__title">{t("memory.quickAdd")}</div>
          <span className="mem-hint">{scopePath}</span>
        </div>
        <div className="mem-add">
          <select className="mem-select" value={activeScope} onChange={(e) => setScope(e.target.value)}>
            {scopes.map((s) => (
              <option key={s.scope} value={s.scope}>{scopeLabel(s.scope, t)}</option>
            ))}
          </select>
          <input
            className="mem-input"
            placeholder={t("memory.notePlaceholder")}
            value={note}
            onChange={(e) => setNote(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") void submitNote(); }}
          />
          <button className="btn btn--primary btn--small" onClick={() => void submitNote()} disabled={busy || !note.trim()}>
            <Plus size={13} className="mem-btn-icon" />{t("memory.remember")}
          </button>
        </div>
      </section>

      {/* tab bar */}
      <div className="mem-tabs">
        {(["saved", "archived", "docs", "suggestions"] as const).map((k) => (
          <button
            key={k}
            className={`mem-tab${tab === k ? " mem-tab--active" : ""}`}
            onClick={() => setTab(k)}
          >
            {{ saved: t("memory.facts"), archived: t("memory.archived"), docs: t("memory.docs"), suggestions: t("memory.suggestions") }[k]}
            {k === "saved" && <span className="mem-count">{facts.length}</span>}
            {k === "archived" && <span className="mem-count">{archives.length}</span>}
            {k === "docs" && <span className="mem-count">{docs.length}</span>}
            {k === "suggestions" && suggestions && <span className="mem-count">{suggestions.memories.length + suggestions.skills.length}</span>}
          </button>
        ))}
      </div>

      {/* search & filter (saved & archived tabs) */}
      {(tab === "saved" || tab === "archived") && (
        <div className="mem-filters">
          <div className="mem-search">
            <Search size={14} className="mem-search__icon" />
            <input
              className="mem-filter"
              placeholder={t("memory.searchPlaceholder")}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              spellCheck={false}
            />
            {query && (
              <button className="mem-search__clear" onClick={() => setQuery("")}><X size={12} /></button>
            )}
          </div>
          <div className="mem-type-chips">
            <button className={`mem-chip${typeFilter === "all" ? " mem-chip--active" : ""}`} onClick={() => setTypeFilter("all")}>
              {t("memory.filterAll")}
            </button>
            {factTypes.map((ft) => (
              <button key={ft} className={`mem-chip${typeFilter === ft ? " mem-chip--active" : ""}`} onClick={() => setTypeFilter(ft)}>
                {ft}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* content */}
      <div className="mem-body">
        {/* ── saved facts ── */}
        {tab === "saved" && (
          <>
            {filteredFacts.length === 0 ? (
              <div className="mem-empty">
                {facts.length === 0 ? t("memory.noFacts") : t("memory.noResults")}
              </div>
            ) : (
              <div className="mem-facts">
                {filteredFacts.map((f) => {
                  const isOpen = expanded === f.name;
                  const links = uniqueLinks(f.body, factNames);
                  return (
                    <article
                      className={`mem-fact${highlight === f.name ? " mem-fact--hl" : ""}`}
                      data-mem-type={f.type || "other"}
                      key={f.name}
                      ref={(el) => { factRefs.current[f.name] = el; }}
                    >
                      <button className="mem-fact__summary" onClick={() => { setExpanded(isOpen ? null : f.name); setConfirmForget(null); }} type="button">
                        {isOpen ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
                        <span className="mem-fact__main">
                          <span className="mem-fact__title">{displayTitle(f)}</span>
                          <span className="mem-fact__meta">
                            {f.type && <span className="mem-fact__type" data-mem-type={f.type}>{factTypeLabel(t, f.type)}</span>}
                            <span className="mem-fact__slug">{f.name}</span>
                          </span>
                          <span className="mem-fact__desc">{f.description}</span>
                        </span>
                      </button>
                      {/* [[links]] chips */}
                      {links.length > 0 && (
                        <div className="mem-fact__links">
                          {links.map((l) =>
                            l.exists ? (
                              <button className="mem-link-chip" key={l.name} onClick={() => jumpTo(l.name)} type="button">[[{l.name}]]</button>
                            ) : (
                              <Tooltip key={l.name} label={t("memory.deadLink", { name: l.name })}>
                                <span className="mem-link-chip mem-link-chip--dead">[[{l.name}]]</span>
                              </Tooltip>
                            )
                          )}
                        </div>
                      )}
                      {isOpen && (
                        <div className="mem-fact__detail">
                          {f.body ? (
                            <div className="mem-fact__body">{renderWithLinks(f.body)}</div>
                          ) : (
                            <div className="mem-empty">{t("memory.noBody")}</div>
                          )}
                          <div className="mem-fact__actions">
                            <button className="btn btn--small" onClick={() => { setDraft(f.body); setEditingPath(`fact:${f.name}`); }} type="button">
                              <Pencil size={12} className="mem-btn-icon" />{t("common.edit")}
                            </button>
                            {confirmForget === f.name ? (
                              <span className="mem-confirm">
                                <span className="text-fg-faint text-[11px] mr-2">{t("memory.confirmForget")}?</span>
                                <button className="btn btn--small" onClick={() => setConfirmForget(null)} disabled={busy}>{t("common.cancel")}</button>
                                <button className="btn btn--small mem-danger" onClick={() => void forgetFact(f.name)} disabled={busy}>{t("memory.forget")}</button>
                              </span>
                            ) : (
                              <button className="btn btn--small mem-fact__forget" onClick={() => setConfirmForget(f.name)} disabled={busy} type="button">
                                <Trash2 size={13} />
                              </button>
                            )}
                          </div>
                        </div>
                      )}
                    </article>
                  );
                })}
              </div>
            )}
            {/* in-place editor */}
            {editingPath && editingPath.startsWith("fact:") && (
              <div className="mem-edit">
                <textarea className="mem-textarea" value={draft} onChange={(e) => setDraft(e.target.value)} spellCheck={false} />
                <div className="mem-edit__actions">
                  <button className="btn btn--small" onClick={() => setEditingPath(null)} disabled={busy}>{t("common.cancel")}</button>
                  <button className="btn btn--primary btn--small" onClick={async () => {
                    const name = editingPath.slice(5);
                    setBusy(true);
                    try { await onSaveFact(name, draft); setEditingPath(null); await reload(); }
                    catch (e) { setError(String(e)); }
                    finally { setBusy(false); }
                  }} disabled={busy}>{t("common.save")}</button>
                </div>
              </div>
            )}
            {/* storage info */}
            {view?.storeDir && (
              <div className="mem-hint mem-hint--footer">{t("memory.storedUnder", { dir: view.storeDir })}</div>
            )}
          </>
        )}

        {/* ── archived ── */}
        {tab === "archived" && (
          <>
            {archives.length === 0 ? (
              <div className="mem-empty">{t("memory.noArchives")}</div>
            ) : filteredArchives.length === 0 ? (
              <div className="mem-empty">{t("memory.noResults")}</div>
            ) : (
              <div className="mem-facts mem-facts--archive">
                {filteredArchives.map((f) => {
                  const key = archiveKey(f);
                  const isOpen = expandedArchive === key;
                  return (
                    <article className="mem-fact mem-fact--archived" data-mem-type={f.type || "other"} key={key}>
                      <button className="mem-fact__summary" onClick={() => setExpandedArchive(isOpen ? null : key)} type="button">
                        {isOpen ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
                        <span className="mem-fact__main">
                          <span className="mem-fact__title">{f.title || f.name}</span>
                          <span className="mem-fact__meta">
                            {f.type && <span className="mem-fact__type" data-mem-type={f.type}>{factTypeLabel(t, f.type)}</span>}
                            <span className="mem-fact__slug">{f.name}</span>
                            {f.archivedAt && <span className="mem-fact__archived">{new Date(f.archivedAt).toLocaleDateString()}</span>}
                          </span>
                          <span className="mem-fact__desc">{f.description}</span>
                        </span>
                      </button>
                      {isOpen && (
                        <div className="mem-fact__detail">
                          {f.body ? <div className="mem-fact__body">{renderWithLinks(f.body)}</div> : <div className="mem-empty">{t("memory.noBody")}</div>}
                          {f.path && <div className="mem-archive__path">{f.path}</div>}
                        </div>
                      )}
                    </article>
                  );
                })}
              </div>
            )}
          </>
        )}

        {/* ── docs ── */}
        {tab === "docs" && (
          <>
            {docs.length === 0 ? (
              <div className="mem-empty">{t("memory.noDocs")}</div>
            ) : filteredDocs.length === 0 ? (
              <div className="mem-empty">{t("memory.noResults")}</div>
            ) : (
              filteredDocs.map((d) => {
                const editing = editingPath === d.path;
                return (
                  <div className="mem-doc" data-doc-scope={d.scope || "other"} key={d.path}>
                    <div className="mem-doc__head">
                      <FileText size={15} className="mem-doc__icon" />
                      <span className="mem-doc__info">
                        <span className="mem-doc__name">{d.path}</span>
                      </span>
                      <span className={`mem-doc__tag mem-doc__tag--${d.scope}`}>{scopeLabel(d.scope, t)}</span>
                      {!editing && (
                        <button className="btn btn--small" onClick={() => startEdit(d.path, d.body)}>{t("common.edit")}</button>
                      )}
                    </div>
                    {editing ? (
                      <div className="mem-doc__edit">
                        <textarea className="mem-textarea" value={draft} onChange={(e) => setDraft(e.target.value)} spellCheck={false} />
                        <div className="mem-edit__actions">
                          <button className="btn btn--small" onClick={() => setEditingPath(null)} disabled={busy}>{t("common.cancel")}</button>
                          <button className="btn btn--primary btn--small" onClick={() => void saveEdit()} disabled={busy}>{t("common.save")}</button>
                        </div>
                      </div>
                    ) : (
                      <pre className="mem-doc__body">{d.body}</pre>
                    )}
                  </div>
                );
              })
            )}
          </>
        )}

        {/* ── suggestions ── */}
        {tab === "suggestions" && (
          <div className="mem-suggestions">
            <div className="mem-suggestions__header">
              <label className="mem-auto-toggle">
                <input type="checkbox" checked={autoSuggestions} onChange={(e) => toggleAuto(e.target.checked)} />
                <span>{t("memory.autoSuggestions")}</span>
              </label>
              <button className="btn btn--small" onClick={() => void refreshSuggestions()} disabled={suggestionBusy}>
                <RefreshCw size={14} className={suggestionBusy ? "animate-spin mem-btn-icon" : "mem-btn-icon"} />
                {suggestions ? t("memory.refreshSuggestions") : t("memory.scanSuggestions")}
              </button>
            </div>

            {!suggestions ? (
              <div className="mem-empty">{t("memory.suggestionsHint")}</div>
            ) : suggestions.memories.length === 0 && suggestions.skills.length === 0 ? (
              <div className="mem-empty">{t("memory.noCandidates")}</div>
            ) : (
              <>
                {suggestions.memories.length > 0 && (
                  <>
                    <div className="mem-section__title">{t("memory.memoryCandidates")}</div>
                    {suggestions.memories.map((s) => (
                      <div className="mem-suggestion" key={s.id || s.name}>
                        <div className="mem-suggestion__info">
                          <span className="mem-suggestion__title">{s.title || s.name}</span>
                          <span className="mem-suggestion__desc">{s.description}</span>
                          <span className="mem-suggestion__reason">{s.reason}</span>
                        </div>
                        {!acceptedSuggestions.has(s.id || s.name) ? (
                          <button className="btn btn--primary btn--small" onClick={async () => { await onAcceptMemorySuggestion(s); setAcceptedSuggestions((prev) => new Set(prev).add(s.id || s.name)); }}>
                            <Check size={13} className="mem-btn-icon" />{t("memory.accept")}
                          </button>
                        ) : (
                          <span className="mem-saved-badge">{t("memory.savedBadge")}</span>
                        )}
                      </div>
                    ))}
                  </>
                )}
                {suggestions.skills.length > 0 && (
                  <>
                    <div className="mem-section__title mt-3">{t("memory.skillCandidates")}</div>
                    {suggestions.skills.map((s) => (
                      <div className="mem-suggestion" key={s.id || s.name}>
                        <div className="mem-suggestion__info">
                          <span className="mem-suggestion__title">{s.name}</span>
                          <span className="mem-suggestion__desc">{s.description}</span>
                          <span className="mem-suggestion__reason">{s.reason}</span>
                        </div>
                        {!acceptedSuggestions.has(s.id || s.name) ? (
                          <button className="btn btn--primary btn--small" onClick={async () => { await onAcceptSkillSuggestion(s); setAcceptedSuggestions((prev) => new Set(prev).add(s.id || s.name)); }}>
                            <Sparkles size={13} className="mem-btn-icon" />{t("memory.create")}
                          </button>
                        ) : (
                          <span className="mem-saved-badge">{t("memory.createdBadge")}</span>
                        )}
                      </div>
                    ))}
                  </>
                )}
                {suggestions.generatedAt && (
                  <div className="mem-hint mem-hint--footer">{t("memory.generatedAt")} {new Date(suggestions.generatedAt).toLocaleString()}</div>
                )}
              </>
            )}
          </div>
        )}
      </div>
    </ResizableDrawer>
  );
}
