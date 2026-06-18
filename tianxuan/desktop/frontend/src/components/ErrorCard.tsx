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
    <div className="error-card">
      <span className="error-card__msg">{item.text}</span>
      <button
        type="button"
        className="error-card__dismiss"
        onClick={() => onDismiss(item.id)}
        aria-label="Dismiss error"
      >
        ✕
      </button>
    </div>
  );
}
