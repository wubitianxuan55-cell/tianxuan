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
    <div
      className="mx-4 my-2 p-2 rounded-lg border border-err border-l-[3px] flex gap-2 items-start"
      style={{ background: "rgba(242,139,130,0.08)" }}
    >
      <span className="flex-1 text-xs text-err leading-snug break-words">{item.text}</span>
      <button
        type="button"
        className="shrink-0 bg-transparent border-0 text-fg-faint cursor-pointer text-sm leading-none px-0.5 hover:text-fg"
        onClick={() => onDismiss(item.id)}
        aria-label="Dismiss error"
      >
        ✕
      </button>
    </div>
  );
}
