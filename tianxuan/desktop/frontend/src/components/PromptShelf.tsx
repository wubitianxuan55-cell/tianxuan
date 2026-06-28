import type { ReactNode } from "react";

/** PromptShelf is a shared inline UI wrapper for transient prompts that appear
 *  between the transcript and the composer — todo lists, undo banners, context
 *  clear confirmations, etc. It provides a consistent visual frame: a subtle
 *  card with title bar, metadata, collapsible body, and action buttons. */

export function PromptShelf(p: {
  titleId?: string;
  title: string;
  badges?: ReactNode;
  meta?: string;
  headerActions?: ReactNode;
  actions?: ReactNode;
  children?: ReactNode;
  role?: string;
  barRef?: React.Ref<HTMLDivElement>;
}) {
  return (
    <div
      ref={p.barRef}
      className="max-w-[--maxw] mx-auto mb-2 border border-border rounded-[9px] bg-bg-soft overflow-hidden"
      style={{ boxShadow: "var(--ds-shadow-card)" }}
      role={p.role}
    >
      {/* Header */}
      <div className="flex items-center gap-2 px-3 py-2">
        <div className="flex items-center gap-2 flex-1 min-w-0">
          <span
            id={p.titleId}
            className="text-fg text-[12.5px] font-semibold shrink-0"
          >
            {p.title}
          </span>
          {p.badges && (
            <span className="flex items-center gap-1">{p.badges}</span>
          )}
          {p.meta && (
            <span className="text-fg-faint text-[11px] truncate">{p.meta}</span>
          )}
        </div>
        {p.headerActions && (
          <div className="flex items-center gap-1 shrink-0">{p.headerActions}</div>
        )}
      </div>

      {/* Body */}
      {p.children && (
        <div className="border-t border-border-soft">{p.children}</div>
      )}

      {/* Footer actions */}
      {p.actions && (
        <div className="flex items-center justify-end gap-2 px-3 py-2 border-t border-border-soft">
          {p.actions}
        </div>
      )}
    </div>
  );
}

/** PromptBadge is a small counter/tag in the shelf header. */
export function PromptBadge(p: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center px-1.5 py-px rounded bg-bg text-fg-faint text-[10px] font-mono tabular-nums">
      {p.children}
    </span>
  );
}

/** PromptHeaderAction is a text button in the shelf header (collapse/close). */
export function PromptHeaderAction(p: {
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      className="border-0 bg-transparent text-fg-faint text-[11px] cursor-pointer px-1.5 py-0.5 rounded hover:text-fg hover:bg-bg transition-colors"
      onClick={p.onClick}
      type="button"
    >
      {p.children}
    </button>
  );
}

/** PromptAction is a numbered action button with keyboard shortcut hint. */
export function PromptAction(p: {
  keyLabel: string;
  label: string;
  selected?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md border text-[12px] cursor-pointer transition-colors ${
        p.selected
          ? "border-accent bg-accent-soft text-accent font-semibold"
          : "border-border-soft bg-transparent text-fg-dim hover:border-border hover:bg-bg-soft"
      }`}
      onClick={p.onClick}
      type="button"
    >
      <span className="inline-flex items-center justify-center w-[18px] h-[18px] rounded border border-current text-[10px] font-mono font-bold">
        {p.keyLabel}
      </span>
      {p.label}
    </button>
  );
}
