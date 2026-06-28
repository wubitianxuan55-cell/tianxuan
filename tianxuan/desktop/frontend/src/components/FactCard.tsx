import { ChevronDown, ChevronRight, Pencil, Trash2 } from "lucide-react";
import { useT } from "../lib/i18n";
import { useState, type ReactNode } from "react";
import type { MemoryFact } from "../lib/types";

function uniqueLinks(body: string, names: Set<string>) {
  const links: { name: string; exists: boolean }[] = [];
  const seen = new Set<string>();
  const re = /\[\[([^\]]+)\]\]/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(body)) !== null) {
    const n = m[1].trim();
    if (!n || seen.has(n)) continue;
    seen.add(n);
    links.push({ name: n, exists: names.has(n) });
  }
  return links;
}

/** renderWithLinks turns [[name]] tokens into clickable in-body jumps. */
function renderWithLinks(
  text: string,
  factNames: Set<string>,
  onJump: (name: string) => void,
): ReactNode[] {
  const out: ReactNode[] = [];
  const re = /\[\[([^\]]+)\]\]/g;
  let last = 0;
  let k = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) out.push(text.slice(last, m.index));
    const target = m[1].trim();
    if (factNames.has(target)) {
      out.push(
        <button
          key={k++}
          type="button"
          className="inline border-0 bg-accent-soft text-accent rounded px-1 py-px cursor-pointer font-mono text-[11px] hover:bg-accent/20 transition-colors"
          onClick={() => onJump(target)}
          title={`跳转到 ${target}`}
        >
          {target}
        </button>,
      );
    } else {
      out.push(
        <span key={k++} className="inline text-fg-faint line-through font-mono text-[11px]" title={`未找到 "${target}"`}>
          {target}
        </span>,
      );
    }
    last = re.lastIndex;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

export function FactCard(p: {
  fact: MemoryFact;
  factNames: Set<string>;
  expanded: boolean;
  highlight: boolean;
  onToggle: () => void;
  onJump: (name: string) => void;
  onForget: () => void;
}) {
  const { fact, factNames, expanded, highlight, onToggle, onJump, onForget } = p;
  const t = useT();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(fact.body);
  const [showDelete, setShowDelete] = useState(false);
  const links = uniqueLinks(fact.body, factNames);

  return (
    <div
      className={`mem-card border rounded-lg overflow-hidden transition-[border-color,box-shadow] duration-120 ${
        highlight
          ? "border-accent shadow-[0_0_0_2px_var(--accent-soft)]"
          : "border-border-soft hover:border-fg-faint/60"
      }`}
    >
      <button
        className="w-full flex items-start gap-2.5 px-3 py-2.5 bg-transparent border-0 text-left cursor-pointer hover:bg-bg-soft/60 transition-colors"
        onClick={onToggle}
        type="button"
      >
        <span className="shrink-0 mt-0.5 text-fg-faint">
          {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </span>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-fg text-[13px] font-medium truncate">
              {fact.title || fact.name}
            </span>
            <span className="badge badge--muted shrink-0">{fact.type}</span>
          </div>
          {fact.description && (
            <div className="text-fg-faint text-[11.5px] leading-relaxed mt-0.5 line-clamp-2">
              {fact.description}
            </div>
          )}
          {!expanded && fact.body && (
            <pre className="m-0 mt-1.5 text-fg-dim/70 text-[11px] leading-relaxed whitespace-pre-wrap line-clamp-3 font-mono border-0 bg-transparent p-0">
              {renderWithLinks(fact.body, factNames, onJump)}
            </pre>
          )}
        </div>
        <div className="flex items-center gap-0.5 shrink-0" onClick={(e) => e.stopPropagation()}>
          <button
            className="w-6 h-6 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint/50 cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors"
            onClick={() => {
              setDraft(fact.body);
              setEditing(true);
            }}
            title={t("common.edit")}
            type="button"
          >
            <Pencil size={13} />
          </button>
          <button
            className="w-6 h-6 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint/50 cursor-pointer hover:text-err hover:bg-bg-soft transition-colors"
            onClick={() => setShowDelete(true)}
            title={t("common.delete")}
            type="button"
          >
            <Trash2 size={13} />
          </button>
        </div>
      </button>

      {expanded && (
        <div className="px-3 pb-3 border-t border-border-soft">
          {editing ? (
            <div className="mt-2">
              <textarea
                className="w-full bg-bg border border-border-soft rounded-md text-fg text-[12px] p-2.5 outline-none resize-y min-h-[100px] focus:border-accent font-mono"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                spellCheck={false}
              />
              <div className="flex justify-end gap-2 mt-1.5">
                <button
                  className="px-2.5 py-1 text-[11px] border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg transition-colors"
                  onClick={() => setEditing(false)}
                  type="button"
                >
                  Cancel
                </button>
                <button
                  className="px-2.5 py-1 text-[11px] border-0 rounded bg-accent text-accent-fg font-semibold cursor-pointer hover:brightness-110 active:scale-[0.97] transition-all"
                  onClick={() => setEditing(false)}
                  type="button"
                >
                  {t("common.save")}
                </button>
              </div>
            </div>
          ) : (
            <pre className="m-0 mt-2 bg-bg-soft border border-border-soft rounded-md p-3 text-fg-dim text-xs leading-relaxed whitespace-pre-wrap max-h-[360px] overflow-y-auto font-mono">
              {renderWithLinks(fact.body, factNames, onJump)}
            </pre>
          )}
          {links.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-1">
              {links.map((l) => (
                <button
                  key={l.name}
                  className={`px-2 py-0.5 rounded text-[10.5px] border cursor-pointer transition-colors ${
                    l.exists
                      ? "border-accent/30 bg-accent-soft text-accent hover:bg-accent/20"
                      : "border-border-soft bg-transparent text-fg-faint line-through hover:bg-bg-soft"
                  }`}
                  onClick={(e) => {
                    e.stopPropagation();
                    if (l.exists) onJump(l.name);
                  }}
                  disabled={!l.exists}
                  type="button"
                  title={l.exists ? `${t("memory.jumpTo")} ${l.name}` : t("memory.deadLink", { name: l.name })}
                >
                  {l.name}
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {showDelete && (
        <div className="px-3 pb-2.5 border-t border-border-soft bg-bg-soft/30">
          <div className="flex items-center gap-3 mt-2">
            <span className="text-fg-faint text-[11px] flex-1">
              {t("memory.confirmForget")} "{fact.title || fact.name}"?
            </span>
            <button
              className="px-2.5 py-1 text-[11px] border border-border-soft rounded bg-transparent text-fg-dim cursor-pointer hover:text-fg transition-colors"
              onClick={() => setShowDelete(false)}
              type="button"
            >
              {t("common.cancel")}
            </button>
            <button
              className="px-2.5 py-1 text-[11px] border-0 rounded bg-err text-white font-semibold cursor-pointer hover:brightness-110 active:scale-[0.97] transition-all"
              onClick={onForget}
              type="button"
            >
              {t("common.delete")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
