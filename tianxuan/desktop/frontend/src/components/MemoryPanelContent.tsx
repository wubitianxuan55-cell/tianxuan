import { Archive, Bookmark, Check, ChevronDown, ChevronRight, FileText, Pencil, Plus, RefreshCw, Search, Sparkles, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { MemoryArchive, MemoryDoc, MemoryFact, MemorySuggestion, MemorySuggestionsView, MemoryView, SkillSuggestion, TabMeta } from "../lib/types";
import { factTypeLabel } from "../lib/factTypeLabel";
import { FactCard } from "./FactCard";
import { Tooltip } from "./Tooltip";

// ── helpers ───────────────────────────────────────────────────────────

function displayTitle(fact: MemoryFact): string {
  return fact.title || fact.name.replaceAll("-", " ");
}

function memoryMatches(fact: MemoryFact | MemoryArchive, normalizedQuery: string, typeFilter: string): boolean {
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

function scopeLabel(scope: string, t: ReturnType<typeof useT>): string {
  return { project: t("memory.scopeProject"), user: t("memory.scopeUser"), local: t("memory.scopeLocal") }[scope] || scope;
}

const AUTO_KEY = "tianxuan-mem-auto-suggestions";
function readAutoPref(): boolean {
  try { return localStorage.getItem(AUTO_KEY) !== "false"; } catch { return true; }
}
function writeAutoPref(v: boolean) {
  try { localStorage.setItem(AUTO_KEY, String(v)); } catch { /* noop */ }
}

export type SubTab = "saved" | "archived" | "docs" | "suggestions";

// ── MemoryPanelContent — 记忆面板核心，drawer 和 settings 两个入口共享 ──

export function MemoryPanelContent() {
  const t = useT();

  // workspace
  const [tabs, setTabs] = useState<TabMeta[]>([]);
  const [selTabId, setSelTabId] = useState<string | null>(null);

  // data
  const [view, setView] = useState<MemoryView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // sub-tab
  const [subTab, setSubTab] = useState<SubTab>("saved");

  // saved / archived filters
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState("all");

  // saved: expand / edit
  const [expandedFact, setExpandedFact] = useState<string | null>(null);
  const [highlightFact, setHighlightFact] = useState<string | null>(null);
  const highlightTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // docs: expand / edit
  const [expandedDoc, setExpandedDoc] = useState<string | null>(null);
  const [editingDoc, setEditingDoc] = useState<string | null>(null);
  const [draftDoc, setDraftDoc] = useState("");

  // quick add
  const [note, setNote] = useState("");
  const [scope, setScope] = useState("");
  const [addOpen, setAddOpen] = useState(false);

  // suggestions
  const [suggestions, setSuggestions] = useState<MemorySuggestionsView | null>(null);
  const [suggestionBusy, setSuggestionBusy] = useState(false);
  const [acceptedSuggestions, setAcceptedSuggestions] = useState<Set<string>>(new Set());
  const [autoSuggestions, setAutoSuggestions] = useState(readAutoPref);
  const autoReq = useRef(false);

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

  // ── load data ──
  const reload = useCallback(async () => {
    const tabId = effectiveTabId;
    setView((prev) => prev && tabId ? { ...prev, facts: [], archives: [], docs: [] } : prev);
    try {
      const v = tabId ? await app.MemoryForTab(tabId) : await app.Memory();
      setView(v);
      setError(null);
    } catch (e) { setView(null); setError(String(e)); }
    finally { setLoading(false); }
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

  const scopePath = useMemo(() => {
    const s = scopes.find((x) => x.scope === activeScope);
    return s?.path ?? "";
  }, [scopes, activeScope]);

  // ── actions ──
  const jumpTo = useCallback((name: string) => {
    if (!factNames.has(name)) return;
    const visible = filteredFacts.some((f) => f.name === name);
    setExpandedFact(name);
    if (!visible) { setQuery(""); setTypeFilter("all"); return; }
    setHighlightFact(name);
    if (highlightTimer.current) clearTimeout(highlightTimer.current);
    highlightTimer.current = setTimeout(() => setHighlightFact((h) => (h === name ? null : h)), 1200);
  }, [factNames, filteredFacts]);

  const submitNote = useCallback(async () => {
    if (!note.trim() || busy) return;
    setBusy(true);
    try {
      await app.Remember(activeScope, note.trim());
      setNote("");
      await reload();
    } catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [note, busy, activeScope, reload]);

  const forgetFact = useCallback(async (name: string) => {
    if (busy) return;
    setBusy(true);
    try {
      if (effectiveTabId) await app.ForgetForTab(effectiveTabId, name);
      else await app.Forget(name);
      await reload();
      if (expandedFact === name) setExpandedFact(null);
    } catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [busy, effectiveTabId, reload, expandedFact]);

  const saveFactBody = useCallback(async (name: string, body: string) => {
    if (busy) return;
    setBusy(true);
    try { await app.UpdateFact(name, body); await reload(); }
    catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [busy, reload]);

  const changeFactType = useCallback(async (name: string, newType: string) => {
    if (busy) return;
    setBusy(true);
    try { await app.ChangeFactType(name, newType); await reload(); }
    catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [busy, reload]);

  const saveDoc = useCallback(async () => {
    if (!editingDoc || busy) return;
    setBusy(true);
    try { await app.SaveDoc(editingDoc, draftDoc); setEditingDoc(null); await reload(); }
    catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [editingDoc, draftDoc, busy, reload]);

  const startEditDoc = useCallback((path: string, body: string) => {
    setEditingDoc(path);
    setDraftDoc(body);
  }, []);

  // suggestions
  const refreshSuggestions = useCallback(async () => {
    if (suggestionBusy) return;
    setSuggestionBusy(true);
    setError(null);
    try {
      const next = effectiveTabId
        ? await app.MemorySuggestionsForTab(effectiveTabId)
        : await app.MemorySuggestions();
      setSuggestions(next);
      setAcceptedSuggestions(new Set());
    } catch (e) { setError(String(e)); }
    finally { setSuggestionBusy(false); }
  }, [effectiveTabId, suggestionBusy]);

  useEffect(() => {
    if (subTab !== "suggestions" || !autoSuggestions || suggestions || suggestionBusy || autoReq.current) return;
    autoReq.current = true;
    void refreshSuggestions();
  }, [autoSuggestions, refreshSuggestions, suggestionBusy, suggestions, subTab]);

  const toggleAuto = useCallback((v: boolean) => { autoReq.current = false; setAutoSuggestions(v); writeAutoPref(v); }, []);

  const acceptMemorySuggestion = useCallback(async (c: MemorySuggestion) => {
    if (busy) return;
    setBusy(true);
    try {
      await app.AcceptMemorySuggestion(c);
      setAcceptedSuggestions((prev) => new Set(prev).add(c.id || c.name));
      await reload();
    } catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [busy, reload]);

  const acceptSkillSuggestion = useCallback(async (c: SkillSuggestion) => {
    if (busy) return;
    setBusy(true);
    try {
      await app.AcceptSkillSuggestion(c);
      setAcceptedSuggestions((prev) => new Set(prev).add(c.id || c.name));
    } catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }, [busy]);

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

  // ── loading / error ──
  if (loading) {
    return <p className="text-[13px] text-fg-faint px-3 py-4">{t("common.loading")}</p>;
  }

  if (error || !view || !view.available) {
    return <p className="text-[13px] text-err px-3 py-4">{error || t("memory.unavailable")}</p>;
  }

  const hasFacts = filteredFacts.length > 0;
  const hasArchives = filteredArchives.length > 0;
  const hasDocs = filteredDocs.length > 0;

  return (
    <>
      {/* ── workspace selector ── */}
      {uniqueTabs.length > 1 && (
        <div className="mem-section">
          <select className="mem-select w-full max-w-[280px]" value={effectiveTabId ?? ""} onChange={(e) => setSelTabId(e.target.value)}>
            {uniqueTabs.map((tb) => (
              <option key={tb.id} value={tb.id}>
                {tb.workspaceName || tb.workspaceRoot || tb.scope}
              </option>
            ))}
          </select>
        </div>
      )}

      {/* ── stats bar ── */}
      {(facts.length > 0 || archives.length > 0 || docs.length > 0) && (
        <div className="flex items-center gap-3 text-[11px] text-fg-faint px-1 mb-1.5">
          {facts.length > 0 && <span><Bookmark size={11} className="inline mr-1 align-[-1px]" />{facts.length} 条记忆</span>}
          {archives.length > 0 && <span><Archive size={11} className="inline mr-1 align-[-1px]" />{archives.length} 条归档</span>}
          {docs.length > 0 && <span><FileText size={11} className="inline mr-1 align-[-1px]" />{docs.length} 个文档</span>}
        </div>
      )}

      {/* ── sub-tabs ── */}
      <div className="mem-tabs">
        {(["saved", "archived", "docs", "suggestions"] as SubTab[]).map((k) => (
          <button
            key={k}
            type="button"
            className={`mem-tab${subTab === k ? " mem-tab--active" : ""}`}
            onClick={() => { setSubTab(k); if (k !== "saved" && k !== "archived") { setQuery(""); setTypeFilter("all"); } }}
          >
            {k === "saved" && <><Bookmark size={13} className="mr-1" />{t("memory.savedMemories")}</>}
            {k === "archived" && <><Archive size={13} className="mr-1" />{t("memory.archived")}</>}
            {k === "docs" && <><FileText size={13} className="mr-1" />{t("memory.instructionFiles")}</>}
            {k === "suggestions" && <><Sparkles size={13} className="mr-1" />{t("memory.suggestions")}</>}
            {k === "saved" && <span className="mem-count">{facts.length}</span>}
            {k === "archived" && <span className="mem-count">{archives.length}</span>}
            {k === "docs" && <span className="mem-count">{docs.length}</span>}
            {k === "suggestions" && suggestions && <span className="mem-count">{suggestions.memories.length + suggestions.skills.length}</span>}
          </button>
        ))}
      </div>

      {/* ── filters (saved + archived) ── */}
      {(subTab === "saved" || subTab === "archived") && (
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
            {query && <button className="mem-search__clear" onClick={() => setQuery("")}><X size={12} /></button>}
          </div>
          <div className="mem-type-chips">
            <button
              className={`mem-chip${typeFilter === "all" ? " mem-chip--active" : ""}`}
              onClick={() => setTypeFilter("all")}
            >{t("memory.allTypes")}</button>
            {factTypes.map((ft) => (
              <button
                key={ft}
                className={`mem-chip${typeFilter === ft ? " mem-chip--active" : ""}`}
                onClick={() => setTypeFilter(ft)}
              >{factTypeLabel(t, ft)}</button>
            ))}
          </div>
        </div>
      )}

      {/* ── body ── */}
      <div className="mem-body">

        {/* ======== SAVED ======== */}
        {subTab === "saved" && (
          <>
            {/* quick add — collapsible */}
            <section className="mem-section">
              <button
                type="button"
                className="flex items-center gap-1.5 w-full text-left bg-transparent border-0 text-fg-faint text-[12px] font-medium py-1 cursor-pointer hover:text-fg transition-colors"
                onClick={() => setAddOpen((v) => !v)}
              >
                {addOpen ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
                {t("memory.quickAdd")}
                <span className="text-fg-faint/50 text-[11px] font-normal">{scopePath}</span>
              </button>
              {addOpen && (
                <div className="mem-add mt-1.5">
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
                  <button className="btn btn--small" onClick={() => void submitNote()} disabled={busy || !note.trim()} type="button">
                    <Plus size={13} className="mem-btn-icon" />{t("memory.remember")}
                  </button>
                </div>
              )}
            </section>

            {!hasFacts && normalizedQuery && (
              <div className="mem-empty">
                <Bookmark size={28} className="mb-2 text-fg-faint/30" />
                <p>{t("memory.noResults")}</p>
              </div>
            )}
            {!hasFacts && !normalizedQuery && (
              <div className="mem-empty">
                <Bookmark size={28} className="mb-2 text-fg-faint/30" />
                <p className="mb-2">{t("memory.noFacts")}</p>
                <button className="btn btn--small" onClick={() => setAddOpen(true)} type="button">
                  <Plus size={13} className="mem-btn-icon" />{t("memory.quickAdd")}
                </button>
              </div>
            )}
            {hasFacts && (
              <div className="flex flex-col gap-1.5">
                {filteredFacts.map((f) => (
                  <FactCard
                    key={f.name}
                    fact={f}
                    factNames={factNames}
                    expanded={expandedFact === f.name}
                    highlight={highlightFact === f.name}
                    onToggle={() => setExpandedFact(expandedFact === f.name ? null : f.name)}
                    onJump={jumpTo}
                    onSave={saveFactBody}
                    onForget={() => void forgetFact(f.name)}
                    onChangeType={changeFactType}
                  />
                ))}
              </div>
            )}
            {error && <p className="text-[12px] text-err mt-2">{error}</p>}
            <div className="mem-hint mem-hint--footer">
              {t("memory.storedUnder", { dir: view.storeDir })}
            </div>
          </>
        )}

        {/* ======== ARCHIVED ======== */}
        {subTab === "archived" && (
          <>
            {!hasArchives && normalizedQuery && (
              <div className="mem-empty">{t("memory.noResults")}</div>
            )}
            {!hasArchives && !normalizedQuery && (
              <div className="mem-empty">
                <Archive size={28} className="mb-2 text-fg-faint/30" />
                <p>{t("memory.noArchives")}</p>
              </div>
            )}
            {hasArchives && (
              <div className="mem-facts mem-facts--archive">
                {filteredArchives.map((f) => {
                  const key = archiveKey(f);
                  const isOpen = expandedFact === key;
                  return (
                    <article className="mem-fact mem-fact--archived" data-mem-type={f.type || "other"} key={key}>
                      <button className="mem-fact__summary" onClick={() => setExpandedFact(isOpen ? null : key)} type="button">
                        {isOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
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

        {/* ======== DOCS ======== */}
        {subTab === "docs" && (
          <>
            {!hasDocs && normalizedQuery && (
              <div className="mem-empty">{t("memory.noResults")}</div>
            )}
            {!hasDocs && !normalizedQuery && (
              <div className="mem-empty">
                <FileText size={28} className="mb-2 text-fg-faint/30" />
                <p>{t("memory.noDocs")}</p>
              </div>
            )}
            {hasDocs && (
              <div className="flex flex-col gap-2">
                {filteredDocs.map((d: MemoryDoc) => {
                  const isOpen = expandedDoc === d.path;
                  const isEditing = editingDoc === d.path;
                  return (
                    <div className="mem-doc" data-doc-scope={d.scope || "other"} key={d.path}>
                      <div className="mem-doc__head">
                        <FileText size={15} className="mem-doc__icon" />
                        <span className="mem-doc__info">
                          <span className="mem-doc__name">{d.path}</span>
                        </span>
                        <span className={`mem-doc__tag mem-doc__tag--${d.scope}`}>{scopeLabel(d.scope, t)}</span>
                        <button
                          className="btn btn--ghost btn--tiny ml-2"
                          onClick={() => { setExpandedDoc(isOpen ? null : d.path); if (isEditing) setEditingDoc(null); }}
                          type="button"
                        >
                          {isOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                        </button>
                        {isOpen && (
                          <button
                            className="btn btn--ghost btn--tiny ml-1"
                            onClick={() => { startEditDoc(d.path, d.body); }}
                            type="button"
                          >
                            <Pencil size={13} />
                          </button>
                        )}
                      </div>
                      {isOpen && (
                        isEditing ? (
                          <div className="mem-doc__edit">
                            <textarea className="mem-textarea" value={draftDoc} onChange={(e) => setDraftDoc(e.target.value)} spellCheck={false} />
                            <div className="mem-edit__actions">
                              <button className="btn btn--small" onClick={() => setEditingDoc(null)} type="button">{t("common.cancel")}</button>
                              <button className="btn btn--small btn--primary" onClick={() => void saveDoc()} disabled={busy} type="button">{t("common.save")}</button>
                            </div>
                          </div>
                        ) : (
                          <pre className="mem-doc__body">{d.body}</pre>
                        )
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </>
        )}

        {/* ======== SUGGESTIONS ======== */}
        {subTab === "suggestions" && (
          <div className="mem-suggestions">
            <div className="mem-suggestions__header">
              <label className="mem-auto-toggle">
                <input type="checkbox" checked={autoSuggestions} onChange={(e) => toggleAuto(e.target.checked)} />
                <span className="text-[12px] text-fg-faint ml-1.5">{t("memory.autoSuggestions")}</span>
              </label>
              <button className="btn btn--small" onClick={() => void refreshSuggestions()} disabled={suggestionBusy} type="button">
                <RefreshCw size={14} className={suggestionBusy ? "animate-spin mem-btn-icon" : "mem-btn-icon"} />
                {suggestionBusy ? t("memory.refreshSuggestions") : t("memory.scanSuggestions")}
              </button>
            </div>

            {error && <p className="text-[12px] text-err mt-2">{error}</p>}

            {!suggestions && !suggestionBusy && (
              <div className="mem-empty">{t("memory.suggestionsHint")}</div>
            )}
            {suggestions && suggestions.memories.length === 0 && suggestions.skills.length === 0 && (
              <div className="mem-empty">{t("memory.noCandidates")}</div>
            )}
            {suggestions && (
              <>
                {suggestions.memories.length > 0 && (
                  <>
                    <div className="mem-section__title mt-2">{t("memory.memoryCandidates")}</div>
                    {suggestions.memories.map((s) => (
                      <div className="mem-suggestion" key={s.id || s.name}>
                        <div className="mem-suggestion__info">
                          <span className="mem-suggestion__title">{s.title || s.name}</span>
                          <span className="mem-suggestion__desc">{s.description}</span>
                          <span className="mem-suggestion__reason">{s.reason}</span>
                        </div>
                        {acceptedSuggestions.has(s.id || s.name) ? (
                          <span className="mem-saved-badge">{t("memory.savedBadge")}</span>
                        ) : (
                          <button className="btn btn--small" onClick={() => void acceptMemorySuggestion(s)} disabled={busy} type="button">
                            <Check size={13} className="mem-btn-icon" />{t("memory.accept")}
                          </button>
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
                        {acceptedSuggestions.has(s.id || s.name) ? (
                          <span className="mem-saved-badge">{t("memory.createdBadge")}</span>
                        ) : (
                          <button className="btn btn--small" onClick={() => void acceptSkillSuggestion(s)} disabled={busy} type="button">
                            <Plus size={13} className="mem-btn-icon" />{t("memory.create")}
                          </button>
                        )}
                      </div>
                    ))}
                  </>
                )}
                {suggestions.generatedAt && (
                  <div className="mem-hint mem-hint--footer">{t("memory.generatedAt")}: {new Date(suggestions.generatedAt).toLocaleString()}</div>
                )}
              </>
            )}
          </div>
        )}
      </div>
    </>
  );
}
