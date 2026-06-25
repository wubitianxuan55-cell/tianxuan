import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";

interface ContextMenuItem {
  id: string;
  label: string;
  icon?: React.ReactNode;
  shortcut?: string;
  disabled?: boolean;
  danger?: boolean;
  onClick: () => void;
  divider?: boolean;
}

interface ContextMenuProps {
  items: ContextMenuItem[];
  position: { x: number; y: number } | null;
  onClose: () => void;
}

export function ContextMenu({ items, position, onClose }: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!position) return;
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof MouseEvent) {
        if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
          onClose();
        }
      } else if (e.key === "Escape") {
        onClose();
      }
    };
    // Use setTimeout to avoid the same click that opened the menu closing it
    const id = setTimeout(() => {
      document.addEventListener("mousedown", close);
      document.addEventListener("keydown", close);
    }, 0);
    return () => {
      clearTimeout(id);
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    };
  }, [position, onClose]);

  if (!position) return null;

  const menuHeight = items.length * 36 + 8;
  const top = Math.min(position.y, window.innerHeight - menuHeight - 8);
  const left = Math.min(position.x, window.innerWidth - 200 - 8);

  return createPortal(
    <div
      ref={menuRef}
      className="context-menu"
      style={{
        position: "fixed",
        top,
        left,
        zIndex: 100,
        minWidth: 180,
        maxWidth: 260,
        background: "var(--bg-elev-2)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius)",
        padding: "4px",
        boxShadow: "var(--ds-shadow-dropdown)",
        animation: "fadeIn 120ms ease-out",
      }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      {items.map((item, i) => (
        <div key={item.id}>
          {item.divider && i > 0 && (
            <div style={{ height: 1, background: "var(--border-soft)", margin: "4px 0" }} />
          )}
          <button
            className="context-menu-item"
            disabled={item.disabled}
            onClick={() => {
              if (!item.disabled) {
                item.onClick();
                onClose();
              }
            }}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              width: "100%",
              padding: "6px 8px",
              border: "none",
              borderRadius: 5,
              background: "transparent",
              color: item.danger ? "var(--err)" : "var(--fg)",
              fontSize: 13,
              cursor: item.disabled ? "default" : "pointer",
              opacity: item.disabled ? 0.4 : 1,
              transition: "background var(--dur-fast)",
            }}
            onMouseEnter={(e) => {
              if (!item.disabled) e.currentTarget.style.background = "var(--sidebar-hover)";
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.background = "transparent";
            }}
          >
            {item.icon && <span style={{ width: 16, height: 16, display: "flex", alignItems: "center" }}>{item.icon}</span>}
            <span style={{ flex: 1, textAlign: "left" }}>{item.label}</span>
            {item.shortcut && (
              <span style={{ fontSize: 11, color: "var(--fg-faint)", marginLeft: "auto" }}>{item.shortcut}</span>
            )}
          </button>
        </div>
      ))}
    </div>,
    document.body,
  );
}

export type { ContextMenuItem, ContextMenuProps };
