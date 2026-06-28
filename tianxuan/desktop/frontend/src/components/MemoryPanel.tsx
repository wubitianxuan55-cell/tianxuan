import { Plus, RefreshCw, Search, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { MemorySuggestion, MemorySuggestionsView, MemoryView, SkillSuggestion } from "../lib/types";
import { DocEditor } from "./DocEditor";
import { useT } from "../lib/i18n";
import { FactCard } from "./FactCard";

export function MemoryPanel(p: {
  view: MemoryView | null;
  onClose: () => void;
  onRemember: (scope: string, note: string) => Promise<void> | void;
  onForget: (name: string) => Promise<void> | void;
  onSaveDoc: (path: string, body: string) => Promise<void> | void;
  onAcceptMemorySuggestion: (candidate: MemorySuggestion) => Promise<void> | void;
  onAcceptSkillSuggestion: (candidate: SkillSuggestion) => Promise<void> | void;
  onRefreshSuggestions: () => Promise<MemorySuggestionsView | null>;
}) {
  const { view, onClose, onRemember, onForget, onSaveDoc, onAcceptMemorySuggestion, onAcceptSkillSuggestion, onRefreshSuggestions } = p;
  const t = useT();
  const [note, setNote] = useState("");
  const [scope, setScope] = useState("");
  const [busy, setBusy] = useState(false);
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState("all");
  const [highlight, setHighlight] = useState<string | null>(null);
  const [expandedFacts, setExpandedFacts] = useState<Set<string>>(new Set());
  const [tab, setTab] = useState<"facts" | "docs" | "suggestions">("facts");
  const factRefs = useRef<Record<string, HTMLElement | null>>({});
  const searchRef = useRef<HTMLInputElement>(null);
  const noteRef = useRef<HTMLInputElement>(null);

  const facts = view?.facts ?? [];
  const docs = view?.docs ?? [];
  const archives = view?.archives ?? [];
  const [suggestions, setSuggestions] = useState<MemorySuggestionsView | null>(null);
  const [suggestionsBusy, setSuggestionsBusy] = useState(false);
  const [acceptedSuggestions, setAcceptedSuggestions] = useState<Set<string>>(new Set());
  const scopes = view?.scopes ?? [];
  const factNames = useMemo(() => new Set(facts.map((f) => f.name)), [facts]);
  const factTypes = useMemo(
    () => Array.from(new Set(facts.map((f) => f.type).filter(Boolean))).sort(),
    [facts],
  );

  const activeScope = scope || scopes[0]?.scope || "";
  if (!scope && scopes.length > 0) setScope(scopes[0].scope);

  const normalizedQuery = query.trim().toLowerCase();
  const filteredFacts = useMemo(
    () =>
      facts.filter((f) => {
        if (typeFilter !== "all" && f.type !== typeFilter) return false;
        if (!normalizedQuery) return true;
        return [f.title, f.name, f.description, f.type, f.body]
          .join(" ")
          .toLowerCase()
          .includes(normalizedQuery);
      }),
    [facts, normalizedQuery, typeFilter],
  );

  const scrollToFact = (name: string) => {
    factRefs.current[name]?.scrollIntoView({ behavior: "smooth", block: "center" });
    setHighlight(name);
    setTimeout(() => setHighlight((h) => (h === name ? null : h)), 1200);
  };

  const jumpTo = (name: string) => {
    setTab("facts");
    const visible = filteredFacts.some((f) => f.name === name);
    if (!visible) {
      setQuery("");
      setTypeFilter("all");
      setTimeout(() => scrollToFact(name), 0);
    } else {
      scrollToFact(name);
    }
  };

  const toggleFact = (name: string) => {
    setExpandedFacts((prev) => {
      const next = new Set(prev);
      next.has(name) ? next.delete(name) : next.add(name);
      return next;
    });
  };

  const submitNote = useCallback(() => {
    if (!note.trim() || busy) return;
    setBusy(true);
    Promise.resolve(onRemember(activeScope, note.trim())).finally(() => {
      setBusy(false);
      setNote("");
    });
  }, [note, busy, activeScope, onRemember]);

  const forgetFact = useCallback(
    (name: string) => {
      setBusy(true);
      Promise.resolve(onForget(name)).finally(() => setBusy(false));
    },
    [onForget],
  );

  // 键盘快捷键
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "/" && document.activeElement === document.body) {
        e.preventDefault();
        searchRef.current?.focus();
        return;
      }
      if (e.ctrlKey && e.key === "n") {
        e.preventDefault();
        noteRef.current?.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="drawer-backdrop" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="drawer drawer--wide" onClick={(e) => e.stopPropagation()}>
        {/* 头部 */}
        <div className="drawer__head">
          <div>
            <div className="drawer__title">{t("memory.title")}</div>
            {view && (
              <div className="drawer__summary">
                {t("memory.summary", { facts: facts.length, docs: docs.length })}
              </div>
            )}
          </div>
          <button className="drawer__close" onClick={onClose} aria-label="Close">
            <X size={18} />
          </button>
        </div>

        {/* 固定顶栏：快速添加 */}
        <div className="shrink-0 px-4 py-3 border-b border-border-soft space-y-2">
          <div className="flex items-center gap-2">
            <select
              className="bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] px-2 py-1.5 outline-none focus:border-accent cursor-pointer"
              value={activeScope}
              onChange={(e) => setScope(e.target.value)}
            >
              {scopes.map((s) => (
                <option key={s.scope} value={s.scope}>
                  {s.scope === "user" ? t("memory.scopeUser") : s.scope === "project" ? t("memory.scopeProject") : s.scope === "local" ? t("memory.scopeLocal") : s.scope}
                </option>
              ))}
            </select>
            <input
              ref={noteRef}
              className="flex-1 bg-bg-soft border border-border-soft rounded-md text-fg text-[12px] px-2.5 py-1.5 outline-none placeholder:text-fg-faint focus:border-accent"
              placeholder={t("memory.notePlaceholder")}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") submitNote();
              }}
            />
            <button
              className="px-3 py-1.5 border-0 rounded-md bg-accent text-accent-fg text-[12px] font-semibold cursor-pointer hover:brightness-110 active:scale-[0.97] transition-all disabled:opacity-40"
              onClick={submitNote}
              disabled={busy || !note.trim()}
              type="button"
            >
              <Plus size={13} className="inline mr-1" />
              {t("common.add")}
            </button>
          </div>
          {scopes.find((s) => s.scope === activeScope)?.path && (
            <div className="text-fg-faint/50 text-[10px] truncate" title={scopes.find((s) => s.scope === activeScope)?.path}>
              保存至: {scopes.find((s) => s.scope === activeScope)?.path}
            </div>
          )}

          {/* 搜索 + 类型过滤（仅事实标签页显示） */}
          {tab === "facts" && (
            <div className="space-y-2">
              <div className="flex items-center gap-1.5 px-2.5 h-8 border border-border rounded-md bg-bg text-fg-faint focus-within:border-accent transition-colors">
                <Search size={14} />
                <input
                  ref={searchRef}
                  className="flex-1 min-w-0 border-0 outline-none bg-transparent text-fg text-[12.5px] placeholder:text-fg-faint"
                  placeholder={t("memory.searchPlaceholder")}
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
              </div>
              <div className="flex items-center gap-1.5 flex-wrap">
                <FilterChip active={typeFilter === "all"} label={t("memory.filterAll")} onClick={() => setTypeFilter("all")} />
                {factTypes.map((ft) => (
                  <FilterChip key={ft} active={typeFilter === ft} label={ft} onClick={() => setTypeFilter(ft)} />
                ))}
              </div>
            </div>
          )}
        </div>

        {/* 标签栏 */}
        <div className="shrink-0 flex border-b border-border-soft">
          <TabButton active={tab === "facts"} onClick={() => setTab("facts")} badge={facts.length}>
            {t("memory.facts")}
          </TabButton>
          <TabButton active={tab === "docs"} onClick={() => setTab("docs")} badge={docs.length}>
            {t("memory.docs")}
          </TabButton>
          <TabButton
            active={tab === "suggestions"}
            onClick={() => setTab("suggestions")}
            badge={suggestions ? suggestions.memories.length + suggestions.skills.length : 0}
          >
            {t("memory.suggestions")}
          </TabButton>
        </div>

        {/* 内容区 */}
        <div className="flex-1 min-h-0 overflow-auto px-4 py-3">
          {tab === "facts" && (
            <>
              {filteredFacts.length === 0 && facts.length === 0 ? (
                <EmptyState message={t("memory.noFacts")} />
              ) : filteredFacts.length === 0 ? (
                <div className="py-10 text-center text-fg-faint text-[13px]">{t("memory.noResults")}</div>
              ) : (
                <div className="flex flex-col gap-2">
                  {filteredFacts.map((fact) => (
                    <div
                      key={fact.name}
                      ref={(el) => {
                        factRefs.current[fact.name] = el;
                      }}
                    >
                      <FactCard
                        fact={fact}
                        factNames={factNames}
                        expanded={expandedFacts.has(fact.name)}
                        highlight={highlight === fact.name}
                        onToggle={() => toggleFact(fact.name)}
                        onJump={jumpTo}
                        onForget={() => forgetFact(fact.name)}
                      />
                    </div>
                  ))}
                </div>
              )}

              {/* 归档 */}
              {archives.length > 0 && (
                <div className="mt-4">
                  <ArchivesSection archives={archives} />
                </div>
              )}
            </>
          )}

          {tab === "docs" && (
            <>
              {docs.length === 0 ? (
                <EmptyState message={t("memory.noDocs")} />
              ) : (
                <DocEditor docs={docs} onSaveDoc={onSaveDoc} busy={busy} />
              )}
            </>
          )}

          {tab === "suggestions" && (
            <div className="flex flex-col gap-3">
              {/* 刷新按钮 */}
              <button
                className="flex items-center justify-center gap-2 px-4 py-2.5 border border-border-soft rounded-lg bg-bg-soft text-fg text-[12.5px] cursor-pointer hover:bg-bg hover:border-accent transition-colors disabled:opacity-40"
                onClick={async () => {
                  setSuggestionsBusy(true);
                  const result = await onRefreshSuggestions();
                  setSuggestions(result);
                  setSuggestionsBusy(false);
                }}
                disabled={suggestionsBusy}
                type="button"
              >
                <RefreshCw size={14} className={suggestionsBusy ? "animate-spin" : ""} />
                {suggestions ? "刷新建议" : "扫描建议"}
              </button>

              {!suggestions ? (
                <EmptyState message="点击扫描来分析最近的对话" />
              ) : suggestions.memories.length === 0 && suggestions.skills.length === 0 ? (
                <EmptyState message="未发现候选项" />
              ) : (
                <>
                  {suggestions.memories.length > 0 && (
                    <div className="text-fg-faint text-[10px] font-semibold uppercase tracking-wider">记忆候选项</div>
                  )}
                  {suggestions.memories.map((s) => (
                    <div key={s.id || s.name} className="border border-border-soft rounded-lg p-3 bg-bg-soft">
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-1.5 mb-1">
                            <span className="text-accent text-[11px] font-semibold uppercase tracking-wide">新建</span>
                            <span className="badge badge--muted">{s.type}</span>
                            {acceptedSuggestions.has(s.id || s.name) && (
                              <span className="text-emerald-400 text-[11px] ml-1">✓ 已保存</span>
                            )}
                          </div>
                          <div className="text-fg text-[12.5px] font-medium">{s.title || s.name}</div>
                          <div className="text-fg-faint text-[11px] mt-0.5">{s.description}</div>
                          <div className="text-fg-faint/70 text-[10px] mt-1">{s.reason}</div>
                          {s.evidence && s.evidence.length > 0 && (
                            <div className="mt-1.5 text-fg-faint/50 text-[10px]">{s.evidence[0]}</div>
                          )}
                        </div>
                        {!acceptedSuggestions.has(s.id || s.name) && (
                          <button
                            className="shrink-0 px-2.5 py-1 text-[11px] font-medium border border-accent rounded text-accent bg-transparent cursor-pointer hover:bg-accent hover:text-accent-fg transition-colors"
                            onClick={async () => {
                              await onAcceptMemorySuggestion(s);
                              setAcceptedSuggestions((prev) => new Set(prev).add(s.id || s.name));
                            }}
                            type="button"
                          >
                            采纳
                          </button>
                        )}
                      </div>
                    </div>
                  ))}
                  {suggestions.skills.length > 0 && (
                    <div className="text-fg-faint text-[10px] font-semibold uppercase tracking-wider mt-2">技能候选项</div>
                  )}
                  {suggestions.skills.map((s) => (
                    <div key={s.id || s.name} className="border border-border-soft rounded-lg p-3 bg-bg-soft">
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-1.5 mb-1">
                            <span className="text-info text-[11px] font-semibold uppercase tracking-wide">新技能</span>
                            {acceptedSuggestions.has(s.id || s.name) && (
                              <span className="text-emerald-400 text-[11px] ml-1">✓ 已创建</span>
                            )}
                          </div>
                          <div className="text-fg text-[12.5px] font-medium">{s.name}</div>
                          <div className="text-fg-faint text-[11px] mt-0.5">{s.description}</div>
                          <div className="text-fg-faint/70 text-[10px] mt-1">{s.reason}</div>
                        </div>
                        {!acceptedSuggestions.has(s.id || s.name) && (
                          <button
                            className="shrink-0 px-2.5 py-1 text-[11px] font-medium border border-accent rounded text-accent bg-transparent cursor-pointer hover:bg-accent hover:text-accent-fg transition-colors"
                            onClick={async () => {
                              await onAcceptSkillSuggestion(s);
                              setAcceptedSuggestions((prev) => new Set(prev).add(s.id || s.name));
                            }}
                            type="button"
                          >
                            创建
                          </button>
                        )}
                      </div>
                    </div>
                  ))}
                  {suggestions.generatedAt && (
                    <div className="text-fg-faint/50 text-[10px] text-right">
                      生成于 {new Date(suggestions.generatedAt).toLocaleString()}
                    </div>
                  )}
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function FilterChip(p: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      className={`ds-chip ${p.active ? "ds-chip--accent" : "ds-chip--muted"}`}
      onClick={p.onClick}
      type="button"
    >
      {p.label}
    </button>
  );
}

function TabButton(p: { active: boolean; onClick: () => void; badge: number; children: string }) {
  return (
    <button
      className={`flex-1 px-4 py-2.5 text-[12.5px] font-medium border-0 bg-transparent cursor-pointer transition-colors border-b-2 ${
        p.active
          ? "text-accent border-accent"
          : "text-fg-faint border-transparent hover:text-fg-dim hover:border-fg-faint/30"
      }`}
      onClick={p.onClick}
      type="button"
    >
      {p.children}
      {p.badge > 0 && (
        <span className="ml-1.5 text-[10px] text-fg-faint">({p.badge})</span>
      )}
    </button>
  );
}

function EmptyState(p: { message: string }) {
  return (
    <div className="py-14 text-center">
      <div className="text-fg-faint/40 text-[13px]">{p.message}</div>
    </div>
  );
}

function ArchivesSection(p: { archives: Array<{ name: string; title?: string; description: string; type: string; path?: string; archivedAt?: string }> }) {
  const st = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        className="flex items-center gap-1.5 text-fg-faint text-[11px] font-semibold uppercase tracking-wider bg-transparent border-0 cursor-pointer hover:text-fg transition-colors"
        onClick={() => setOpen((v) => !v)}
        type="button"
      >
        {open ? "▾" : "▸"} {st("memory.archived")}
        <span className="text-fg-faint/60 font-normal">({p.archives.length})</span>
      </button>
      {open && (
        <div className="mt-2 flex flex-col gap-1.5">
          {p.archives.map((a) => (
            <div
              key={a.name}
              className="border border-border-soft rounded-md px-3 py-2 bg-bg-soft/50 opacity-70 hover:opacity-100 transition-opacity"
            >
              <div className="flex items-center gap-2">
                <span className="text-fg-dim text-[12px] font-medium">{a.title || a.name}</span>
                <span className="badge badge--muted">{a.type}</span>
                {a.archivedAt && (
                  <span className="text-fg-faint text-[10px] ml-auto font-mono">
                    {new Date(a.archivedAt).toLocaleDateString()}
                  </span>
                )}
              </div>
              {a.description && (
                <div className="text-fg-faint text-[10.5px] mt-0.5">{a.description}</div>
              )}
            </div>
          ))}
        </div>
      )}
    </>
  );
}
