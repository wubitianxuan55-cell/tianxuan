import { memo } from "react";
import type { MemorySuggestion, SkillSuggestion } from "../lib/types";

export const SuggestionCard = memo(function SuggestionCard(p: {
  item: MemorySuggestion | SkillSuggestion;
  accepted: boolean;
  badge: string;
  acceptedBadge: string;
  actionLabel: string;
  onAccept: () => Promise<void>;
}) {
  const { item, accepted, badge, acceptedBadge, actionLabel, onAccept } = p;
  const name = "name" in item ? item.name : "";
  const title = "title" in item ? (item.title || item.name) : name;
  const type = "type" in item ? item.type : undefined;

  return (
    <div className="border border-border-soft rounded-xl p-3.5 bg-bg-soft/60 hover:bg-bg-soft transition-colors">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5 mb-1.5">
            <span className="text-accent text-[10px] font-semibold uppercase tracking-wider bg-accent/10 px-1.5 py-0.5 rounded">
              {badge}
            </span>
            {type && <span className="badge badge--muted">{type}</span>}
            {accepted && (
              <span className="text-emerald-400 text-[10px] font-medium ml-1">{acceptedBadge}</span>
            )}
          </div>
          <div className="text-fg text-[12.5px] font-medium">{title}</div>
          <div className="text-fg-faint text-[11px] mt-0.5">{item.description}</div>
          <div className="text-fg-faint/60 text-[10px] mt-1 italic">{item.reason}</div>
          {item.evidence && item.evidence.length > 0 && (
            <div className="mt-1.5 text-fg-faint/40 text-[10px] leading-relaxed border-l-2 border-fg-faint/15 pl-2">
              {item.evidence[0]}
            </div>
          )}
        </div>
        {!accepted && (
          <button
            className="shrink-0 px-3 py-1 text-[11px] font-medium border border-accent/50 rounded-lg text-accent bg-transparent cursor-pointer hover:bg-accent hover:text-accent-fg transition-colors"
            onClick={onAccept}
            type="button"
          >
            {actionLabel}
          </button>
        )}
      </div>
    </div>
  );
});
