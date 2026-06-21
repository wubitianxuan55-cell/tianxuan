import { X } from "lucide-react";
import type { Item } from "../lib/store";

/**
 * ErrorCard — a dismissible error display for turn_done failures.
 * Renders a red-bordered card with the error message and a close button.
 */
export function ErrorCard({
  item,
  onDismiss,
}: {
  item: Extract<Item, { kind: "notice" }>;
  onDismiss: (id: string) => void;
}) {
  return (
    <div className="mx-4 my-2 p-2 rounded-lg border border-[color-mix(in_srgb,var(--ds-danger)_30%,transparent)] border-l-[3px] border-l-err flex gap-2 items-start" style={{background: "var(--ds-danger-soft)"}}>
      <span className="flex-1 text-xs text-err leading-snug break-words">{item.text}</span>
      <button
        type="button"
        className="shrink-0 bg-transparent border-0 text-fg-faint cursor-pointer p-0.5 rounded hover:text-err hover:bg-bg-soft transition-colors"
        onClick={() => onDismiss(item.id)}
        aria-label="Dismiss error"
      >
        <X size={14} />
      </button>
    </div>
  );
}
