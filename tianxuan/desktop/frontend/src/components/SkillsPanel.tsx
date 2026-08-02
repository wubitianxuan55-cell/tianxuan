import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Blocks, ChevronDown, Copy, Search } from "lucide-react";
import { app } from "../lib/bridge";
import type { SkillView } from "../lib/types";
import { useToast } from "./Toast";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import { highlightParts } from "../lib/toolCatalog";

const SCOPE_LABEL: Record<string, string> = {
  builtin: "内置",
  project: "项目",
  global: "全局",
};
const SCOPE_ORDER = ["builtin", "project", "global"];

function Highlight({ text, query, className }: { text: string; query: string; className?: string }) {
  const parts = highlightParts(text, query);
  return (
    <span className={className}>
      {parts.map((p, i) =>
        p.hit ? (
          <mark key={i} className="bg-accent/25 text-accent rounded-[2px] px-px">
            {p.text}
          </mark>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </span>
  );
}

function SkillCard({
  sk,
  count,
  query,
  onCopy,
}: {
  sk: SkillView;
  count: number;
  query: string;
  onCopy: (name: string) => void;
}) {
  const active = count > 0;
  const isSubagent = sk.runAs === "subagent";
  return (
    <button
      type="button"
      title={sk.description}
      onClick={() => onCopy(sk.name)}
      className={`group flex items-start gap-1.5 w-full px-2 py-1.5 rounded-md border border-border-soft bg-bg cursor-pointer text-left transition-colors ${
        active ? "border-accent-soft bg-sidebar-active" : "hover:border-accent-soft hover:bg-sidebar-active"
      }`}
    >
      <span className={`w-1.5 h-1.5 mt-[5px] rounded-full shrink-0 ${active ? "bg-accent" : "bg-border-soft"}`} />
      <span className="flex-1 min-w-0 flex flex-col gap-0.5 leading-[1.25]">
        <span className="flex items-center gap-1 min-w-0">
          <Highlight
            text={sk.name}
            query={query}
            className={`font-mono text-[10.5px] truncate ${active ? "text-accent font-semibold" : "text-fg-dim"}`}
          />
          {isSubagent && (
            <span className="shrink-0 text-[9px] px-1 py-px rounded bg-accent-soft text-accent font-medium">🤖 子代理</span>
          )}
        </span>
        <Highlight
          text={sk.description}
          query={query}
          className="text-[10px] text-fg-faint leading-[1.3] line-clamp-2"
        />
      </span>
      <span className={`shrink-0 font-mono text-[11px] font-semibold mt-px ${active ? "text-accent" : "text-fg-faint"}`}>
        {count}
      </span>
      <Copy
        size={10}
        className="shrink-0 self-center text-fg-faint opacity-0 group-hover:opacity-100 transition-opacity"
      />
    </button>
  );
}

function SkillGroup({
  label,
  skills,
  counts,
  query,
  defaultOpen,
  onCopy,
}: {
  label: string;
  skills: SkillView[];
  counts: Record<string, number>;
  query: string;
  defaultOpen: boolean;
  onCopy: (name: string) => void;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const ref = useRef<HTMLDivElement>(null);
  useGSAPCollapse(ref, open, { duration: 0.18 });

  if (skills.length === 0) return null;
  return (
    <div className="px-1.5 py-0.5">
      <button
        className="flex items-center gap-1 w-full px-1 py-1.5 bg-transparent border-0 text-left cursor-pointer hover:bg-bg-soft rounded transition-colors"
        onClick={() => setOpen((v) => !v)}
      >
        <ChevronDown
          size={10}
          className={`text-fg-faint transition-transform duration-150 ${open ? "rotate-0" : "-rotate-90"}`}
        />
        <span className="text-[10px] font-semibold uppercase tracking-[0.5px] text-fg-faint">{label}</span>
        <span className="ml-auto text-[9px] font-mono text-fg-faint/50">{skills.length}</span>
      </button>
      <div ref={ref} style={{ overflow: "hidden" }}>
        <div className="flex flex-col gap-0.5 pt-0.5 pb-1">
          {skills.map((sk) => (
            <SkillCard key={sk.name} sk={sk} count={counts[sk.name] ?? 0} query={query} onCopy={onCopy} />
          ))}
        </div>
      </div>
    </div>
  );
}

/** SkillsPanel — right drawer 技能 tab: discovered skills grouped by scope,
 *  with search highlighting, used-first ordering, an "only used" filter and
 *  one-click copy of the skill name. */
export function SkillsPanel({ counts }: { counts: Record<string, number> }) {
  const toast = useToast();
  const [skills, setSkills] = useState<SkillView[]>([]);
  const [query, setQuery] = useState("");
  const [onlyUsed, setOnlyUsed] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    app
      .Capabilities()
      .then((v) => setSkills(v.skills ?? []))
      .catch(() => setSkills([]));
  }, []);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    let list = skills;
    if (onlyUsed) {
      list = list.filter((sk) => (counts[sk.name] ?? 0) > 0);
    }
    if (q) {
      list = list.filter((sk) => sk.name.toLowerCase().includes(q) || sk.description.toLowerCase().includes(q));
    }
    // Used skills first; stable otherwise.
    return [...list].sort((a, b) => ((counts[b.name] ?? 0) > 0 ? 1 : 0) - ((counts[a.name] ?? 0) > 0 ? 1 : 0));
  }, [skills, query, onlyUsed, counts]);

  const grouped = useMemo(() => {
    const map = new Map<string, SkillView[]>();
    for (const sk of filtered) {
      const arr = map.get(sk.scope) ?? [];
      arr.push(sk);
      map.set(sk.scope, arr);
    }
    const result: { scope: string; label: string; skills: SkillView[] }[] = [];
    for (const scope of SCOPE_ORDER) {
      const items = map.get(scope);
      if (items && items.length > 0) {
        result.push({ scope, label: SCOPE_LABEL[scope] ?? scope, skills: items });
      }
    }
    for (const [scope, items] of map) {
      if (!SCOPE_ORDER.includes(scope)) {
        result.push({ scope, label: scope, skills: items });
      }
    }
    return result;
  }, [filtered]);

  const hasResults = filtered.length > 0;

  const copyName = useCallback(
    (name: string) => {
      navigator.clipboard?.writeText(name).then(
        () => toast.show(`已复制 ${name}`, "info"),
        () => {},
      );
    },
    [toast],
  );

  return (
    <div className="flex flex-col overflow-hidden text-xs h-full">
      {/* Header */}
      <div className="flex items-center gap-1.5 px-2.5 py-2 border-b border-border-soft text-fg-dim font-semibold text-[11px] shrink-0">
        <Blocks size={12} />
        <span>技能</span>
        <span className="ml-auto text-[10px] font-mono text-fg-faint/50">{skills.length}</span>
      </div>

      {/* Search + only-used filter */}
      <div className="flex items-center gap-1.5 mx-2 my-1.5 shrink-0">
        <div className="flex items-center gap-1.5 flex-1 px-2 h-7 border border-border rounded-md bg-bg text-fg-faint">
          <Search size={12} />
          <input
            ref={inputRef}
            className="flex-1 min-w-0 border-0 outline-none bg-transparent text-fg text-[11.5px] placeholder:text-fg-faint"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索技能…"
          />
          {query && (
            <button
              className="border-0 bg-transparent text-fg-faint cursor-pointer p-0 leading-none hover:text-fg"
              onClick={() => {
                setQuery("");
                inputRef.current?.focus();
              }}
            >
              ✕
            </button>
          )}
        </div>
        <button
          className={`h-7 px-2 rounded-md border text-[10.5px] font-medium cursor-pointer transition-colors ${
            onlyUsed
              ? "border-accent bg-accent/10 text-accent"
              : "border-border text-fg-faint hover:text-fg hover:border-border-strong"
          }`}
          onClick={() => setOnlyUsed((v) => !v)}
          title="只显示本次会话用过的技能"
        >
          仅已用
        </button>
      </div>

      {/* Skill list */}
      <div className="flex-1 min-h-0 overflow-y-auto pb-2">
        {skills.length === 0 ? (
          <div className="empty-state">加载中…</div>
        ) : !hasResults ? (
          <div className="empty-state">无匹配技能</div>
        ) : (
          grouped.map((g) => (
            <SkillGroup
              key={g.scope}
              label={g.label}
              skills={g.skills}
              counts={counts}
              query={query}
              defaultOpen={g.scope === "builtin" || g.skills.some((sk) => sk.runAs === "subagent") || grouped.length === 1}
              onCopy={copyName}
            />
          ))
        )}
      </div>
    </div>
  );
}
