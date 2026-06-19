import { ChevronDown, ChevronRight, Search, Trash2 } from "lucide-react";
import { useMemo, useRef, useState, type ReactNode } from "react";
import { useT } from "../lib/i18n";
import type { MemoryFact, MemoryView } from "../lib/types";
import { ResizableDrawer } from "./ResizableDrawer";

type LinkInfo = {
  name: string;
  exists: boolean;
};

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

// MemoryPanel is the desktop memory manager: a right-side drawer over the loaded
// REASONIX.md hierarchy and saved auto-memories. Unlike Claude Code's /memory
// (which shells out to $EDITOR) it edits docs in place, and unlike Codex (no UI
// at all) it shows the saved facts. Docs are editable; facts are read-only
// (the model owns them via the `remember` tool). Quick-add mirrors the "#"
// shortcut with an explicit scope selector.
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
          .join(" ")
          .toLowerCase()
          .includes(normalizedQuery);
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

  // Clear active filters when the target is hidden, else the [[link]] is a silent no-op.
  const jumpTo = (name: string) => {
    if (!factNames.has(name)) return;
    const visible = filteredFacts.some((f) => f.name === name);
    setExpanded(name);
    setConfirmForget(null);
    if (!visible) {
      setQuery("");
      setTypeFilter("all");
      window.setTimeout(() => scrollToFact(name), 0);
      return;
    }
    scrollToFact(name);
  };

  // renderWithLinks turns [[name]] tokens into in-panel jumps; a token with no
  // matching saved memory renders as a flagged dead link.
  const renderWithLinks = (text: string): ReactNode[] => {
    const out: ReactNode[] = [];
    const re = /\[\[([^\]]+)\]\]/g;
    let last = 0;
    let k = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(text)) !== null) {
      if (m.index > last) out.push(text.slice(last, m.index));
      const target = m[1].trim();
      out.push(
        factNames.has(target) ? (
          <button key={k++} type="button" className="mem-link" onClick={() => jumpTo(target)}>
            {target}
          </button>
        ) : (
          <span
            key={k++}
            className="mem-link mem-link--dead"
            title={t("memory.deadLink", { name: target })}
          >
            {target}
          </span>
        ),
      );
      last = re.lastIndex;
    }
    if (last < text.length) out.push(text.slice(last));
    return out;
  };

  const forgetFact = async (name: string) => {
    if (busy) return;
    setBusy(true);
    try {
      await onForget(name);
      if (expanded === name) setExpanded(null);
      setConfirmForget(null);
    } finally {
      setBusy(false);
    }
  };

  const scopes = view?.scopes ?? [];
  // Default the scope selector to "project" when present, else the first option.
  const activeScope =
    scope || scopes.find((s) => s.scope === "project")?.scope || scopes[0]?.scope || "project";

  const submitNote = async () => {
    const trimmed = note.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    try {
      await onRemember(activeScope, trimmed);
      setNote("");
    } finally {
      setBusy(false);
    }
  };

  const startEdit = (path: string, body: string) => {
    setEditingPath(path);
    setDraft(body);
  };

  const saveEdit = async () => {
    if (editingPath === null || busy) return;
    setBusy(true);
    try {
      await onSaveDoc(editingPath, draft);
      setEditingPath(null);
    } finally {
      setBusy(false);
    }
  };

  return (
    <ResizableDrawer onClose={onClose}>
        <header className="drawer__head">
          <div>
            <div className="drawer__title">{t("memory.title")}</div>
            {view?.available && (
              <div className="drawer__summary">
                {t("memory.summary", { facts: facts.length, docs: view.docs.length })}
              </div>
            )}
          </div>
          <button className="chip" onClick={onClose} title={t("common.close")}>
            ✕
          </button>
        </header>

        {!view?.available ? (
          <div className="py-5 text-fg-faint text-sm text-center">{t("memory.unavailable")}</div>
        ) : (
          <div className="drawer__body">
            <section className="mb-3">
              <div className="flex items-center justify-between px-2 pb-1.5">
                <div>
                  <div className="text-fg text-sm font-semibold">{t("memory.savedMemories")}</div>
                  <div className="text-fg-faint text-[11px]">{t("memory.fallibleNote")}</div>
                </div>
                <span className="bg-bg-elev-2 text-fg-dim text-[11px] font-mono px-2 py-0.5 rounded-full">{facts.length}</span>
              </div>
              <div className="flex items-center gap-2 mb-2 px-1">
                <label className="flex items-center gap-1.5 flex-1 px-2.5 h-8 border border-border rounded-md bg-bg-soft text-fg-faint focus-within:border-accent">
                  <Search size={14} />
                  <input
                    className="flex-1 border-0 outline-none bg-transparent text-fg text-[13px] placeholder:text-fg-faint"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder={t("memory.searchPlaceholder")}
                  />
                </label>
                <div className="flex gap-1" role="tablist" aria-label={t("memory.typeFilter")}>
                  <button className={`px-2.5 py-1 border border-border-soft rounded-md text-[11px] bg-transparent cursor-pointer hover:bg-bg-soft hover:text-fg ${typeFilter==="all"?"bg-accent-soft text-accent border-accent/30":"text-fg-dim"}`} onClick={()=>setTypeFilter("all")} type="button">{t("memory.allTypes")}</button>
                  {factTypes.map((type)=><button className={`px-2.5 py-1 border border-border-soft rounded-md text-[11px] bg-transparent cursor-pointer hover:bg-bg-soft hover:text-fg ${typeFilter===type?"bg-accent-soft text-accent border-accent/30":"text-fg-dim"}`} onClick={()=>setTypeFilter(type)} type="button" key={type}>{type}</button>)}
                </div>
              </div>
              {facts.length === 0 ? (
                <div className="py-4 text-fg-faint text-xs text-center">{t("memory.noFacts")}</div>
              ) : filteredFacts.length === 0 ? (
                <div className="py-4 text-fg-faint text-xs text-center">
                  {t("memory.noMatches")}
                  <button
                    className="mt-2 px-3 py-1 border border-border rounded text-fg-dim text-[11px] bg-transparent cursor-pointer hover:bg-bg-soft hover:text-fg"
                    onClick={() => { setQuery(""); setTypeFilter("all"); }}
                    type="button"
                  >
                    {t("memory.clearFilters")}
                  </button>
                </div>
              ) : (
                <div className="mem-facts">
                  {filteredFacts.map((f) => {
                    const isOpen = expanded === f.name;
                    const links = uniqueLinks(f.body, factNames);
                    const missing = links.filter((link) => !link.exists);
                    return (
                      <article
                        className={`border border-border-soft rounded-lg overflow-hidden mb-1.5 ${
                          highlight === f.name ? "ring-2 ring-accent/30" : ""
                        }`}
                        key={f.name}
                        ref={(el) => { factRefs.current[f.name] = el; }}
                      >
                        <button
                          className="flex items-start gap-2 w-full px-2.5 py-2 border-0 bg-transparent text-left cursor-pointer hover:bg-bg-soft"
                          onClick={() => { setExpanded(isOpen ? null : f.name); setConfirmForget(null); }}
                          type="button"
                        >
                          {isOpen ? <ChevronDown size={15} className="shrink-0 mt-0.5 text-fg-faint" /> : <ChevronRight size={15} className="shrink-0 mt-0.5 text-fg-faint" />}
                          <span className="flex-1 min-w-0 flex flex-col gap-0.5">
                            <span className="text-fg text-[13px] font-semibold leading-tight">{displayTitle(f)}</span>
                            <span className="text-fg-faint font-mono text-[10px]">{f.name} · {f.type}</span>
                            <span className="text-fg-dim text-[11.5px] leading-snug">{f.description}</span>
                          </span>
                        </button>
                        {links.length > 0 && (
                          <div className="flex flex-wrap gap-1 px-2.5 pb-1" aria-label={t("memory.links")}>
                            {links.map((link) =>
                              link.exists ? (
                                <button
                                  className="inline-flex items-center px-2 py-0.5 border border-accent/20 rounded text-accent text-[11px] font-mono bg-transparent cursor-pointer hover:bg-accent-soft"
                                  key={link.name}
                                  onClick={() => jumpTo(link.name)}
                                  type="button"
                                >[[{link.name}]]</button>
                              ) : (
                                <span
                                  className="inline-flex items-center px-2 py-0.5 border border-border-soft rounded text-fg-faint text-[11px] font-mono opacity-60"
                                  key={link.name}
                                  title={t("memory.deadLink", { name: link.name })}
                                >[[{link.name}]]</span>
                              ),
                            )}
                          </div>
                        )}
                        {isOpen && (
                          <div className="border-t border-border-soft px-2.5 py-2">
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
                                  <button className="btn btn--small" onClick={() => setConfirmForget(null)} disabled={busy} type="button">{t("common.cancel")}</button>
                                  <button className="btn btn--small mem-danger" onClick={() => void forgetFact(f.name)} disabled={busy} type="button">{t("memory.confirmForget")}</button>
                                </div>
                              ) : (
                                <button className="btn btn--small mem-fact__forget" onClick={() => setConfirmForget(f.name)} disabled={busy} type="button"><Trash2 size={13} />{t("memory.forget")}</button>
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
                <div className="mt-2 text-fg-faint text-[10px] px-1" title={view.storeDir}>
                  {t("memory.storedUnder", { dir: view.storeDir })}
                </div>
              )}
            </section>

            {/* Quick-add */}
            <section className="mb-3">
              <div className="text-fg text-sm font-semibold mb-1.5 px-1">{t("memory.quickAdd")}</div>
              <div className="flex items-center gap-2">
                <select
                  className="bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2 py-1.5 outline-none focus:border-accent"
                  value={activeScope}
                  onChange={(e) => setScope(e.target.value)}
                  title={t("memory.whereToSave")}
                >
                  {scopes.map((s) => (<option key={s.scope} value={s.scope}>{s.scope}</option>))}
                </select>
                <input
                  className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[13px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
                  placeholder={t("memory.notePlaceholder")}
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  onKeyDown={(e) => { if (e.key === "Enter") void submitNote(); }}
                />
                <button
                  className="btn btn--primary btn--small"
                  onClick={() => void submitNote()}
                  disabled={busy || !note.trim()}
                >
                  {t("memory.remember")}
                </button>
              </div>
              <div className="text-fg-faint text-[10px] mt-1 px-1">
                {scopes.find((s) => s.scope === activeScope)?.path}
              </div>
            </section>

            {/* Doc files */}
            <section className="mb-3">
              <div className="text-fg text-sm font-semibold mb-1.5 px-1">{t("memory.instructionFiles")}</div>
              {view.docs.length === 0 && (
                <div className="py-4 text-fg-faint text-xs text-center">{t("memory.noDocs")}</div>
              )}
              {view.docs.map((d) => {
                const editing = editingPath === d.path;
                return (
                  <div className="mb-2 border border-border-soft rounded-lg overflow-hidden" key={d.path}>
                    <div className="flex items-center gap-2 px-2.5 py-1.5">
                      <span className={`badge badge--${d.scope}`}>{d.scope}</span>
                      <span className="flex-1 text-fg-dim font-mono text-[11px] truncate" title={d.path}>{d.path}</span>
                      {!editing && (
                        <button className="btn btn--small" onClick={() => startEdit(d.path, d.body)}>{t("common.edit")}</button>
                      )}
                    </div>
                    {editing ? (
                      <div className="px-2.5 pb-2">
                        <textarea
                          className="w-full bg-bg border border-border-soft rounded-md text-fg text-[13px] p-2 outline-none resize-y min-h-[120px] focus:border-accent"
                          value={draft}
                          onChange={(e) => setDraft(e.target.value)}
                          spellCheck={false}
                        />
                        <div className="flex justify-end gap-2 mt-1.5">
                          <button className="btn btn--small" onClick={() => setEditingPath(null)} disabled={busy}>{t("common.cancel")}</button>
                          <button className="btn btn--primary btn--small" onClick={() => void saveEdit()} disabled={busy}>{t("common.save")}</button>
                        </div>
                      </div>
                    ) : (
                      <pre className="m-0 px-2.5 py-2 bg-bg text-fg-dim text-xs leading-relaxed whitespace-pre-wrap border-t border-border-soft">{d.body}</pre>
                    )}
                  </div>
                );
              })}
            </section>
          </div>
        )}
    </ResizableDrawer>
  );
}
