import { useState } from "react";
import { ChevronDown, Info, TriangleAlert, X } from "lucide-react";
import type { Item } from "../lib/store";

type NoticeItem = Extract<Item, { kind: "notice" }>;

/**
 * ErrorCard — dismissible notice card with icon, optional expandable detail.
 * Pattern aligned with DeepSeek-Reasonix NoticeCard.
 */
export function ErrorCard({
  item,
  onDismiss,
}: {
  item: NoticeItem;
  onDismiss: (id: string) => void;
}) {
  const [detailOpen, setDetailOpen] = useState(false);
  const lines = item.text.split("\n");
  const title = lines.length > 1 ? lines[0] : undefined;
  const body = title ? lines.slice(1).join("\n").trim() : item.text;
  const hasDetail = body.length > 200;
  const Icon = item.level === "warn" ? TriangleAlert : Info;

  return (
    <div className={`notice-line notice-line--${item.level}`} data-entrance="true">
      <Icon className="notice-line__icon" size={14} aria-hidden="true" />
      <div className="notice-line__text">
        {title && <div className="notice-line__title">{title}</div>}
        <div className="notice-line__body">
          {hasDetail && !detailOpen ? `${body.slice(0, 200)}…` : body}
        </div>
        {hasDetail && (
          <button
            type="button"
            className="notice-line__detail-toggle"
            onClick={() => setDetailOpen((v) => !v)}
            aria-expanded={detailOpen}
          >
            <ChevronDown
              className={`notice-line__detail-chevron${detailOpen ? " notice-line__detail-chevron--open" : ""}`}
              size={12}
              aria-hidden="true"
            />
            <span>{detailOpen ? "收起详情" : "显示详情"}</span>
          </button>
        )}
      </div>
      <button
        type="button"
        className="notice-line__dismiss"
        onClick={() => onDismiss(item.id)}
        aria-label="Dismiss"
      >
        <X size={13} />
      </button>
    </div>
  );
}
