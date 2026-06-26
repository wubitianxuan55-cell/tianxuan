import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent, ReactNode } from "react";
import { createPortal } from "react-dom";

export type ContextMenuPoint = { left: number; top: number };

export type ContextMenuItem =
  | {
      type?: "item";
      key: string;
      icon?: ReactNode;
      label: ReactNode;
      disabled?: boolean;
      danger?: boolean;
      variant?: "section";
      onSelect: () => void;
    }
  | {
      type: "separator";
      key: string;
    };

const EDGE_GAP = 8;

function clampMenuPoint(left: number, top: number, width: number, height: number): ContextMenuPoint {
  if (typeof window === "undefined") return { left, top };
  return {
    left: Math.min(Math.max(EDGE_GAP, left), Math.max(EDGE_GAP, window.innerWidth - width - EDGE_GAP)),
    top: Math.min(Math.max(EDGE_GAP, top), Math.max(EDGE_GAP, window.innerHeight - height - EDGE_GAP)),
  };
}

/** 从鼠标/键盘事件中提取右键菜单的锚点坐标。 */
export function contextMenuPointFromEvent(
  event: ReactMouseEvent<HTMLElement> | ReactKeyboardEvent<HTMLElement>,
): ContextMenuPoint {
  if ("clientX" in event && event.clientX > 0 && event.clientY > 0) {
    return { left: event.clientX, top: event.clientY };
  }
  const rect = event.currentTarget.getBoundingClientRect();
  return { left: rect.left + 12, top: rect.bottom + 6 };
}

/** 右键菜单组件——Portal 渲染，键盘导航，边缘裁剪。 */
export function ContextMenu({
  open,
  point,
  items,
  onClose,
  minWidth = 180,
  ariaLabel = "Context menu",
}: {
  open: boolean;
  point: ContextMenuPoint | null;
  items: ContextMenuItem[];
  onClose: () => void;
  minWidth?: number;
  ariaLabel?: string;
}) {
  const menuRef = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState<ContextMenuPoint | null>(point);

  useLayoutEffect(() => {
    if (!open || !point) return;
    const rect = menuRef.current?.getBoundingClientRect();
    if (!rect) {
      setPosition(point);
      return;
    }
    setPosition(clampMenuPoint(point.left, point.top, rect.width, rect.height));
  }, [open, point, items]);

  useEffect(() => {
    if (!open) return;
    const closeOnOutsidePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (target instanceof Node && menuRef.current?.contains(target)) return;
      onClose();
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("pointerdown", closeOnOutsidePointerDown);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsidePointerDown);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open, onClose]);

  const focusIndexRef = useRef(-1);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      const actionItems = items.filter((item): item is Extract<ContextMenuItem, { type?: "item" }> => item.type !== "separator" && !("disabled" in item && item.disabled));
      if (actionItems.length === 0) return;
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        const dir = event.key === "ArrowDown" ? 1 : -1;
        focusIndexRef.current = ((focusIndexRef.current + dir) % actionItems.length + actionItems.length) % actionItems.length;
        const btn = itemRefs.current[focusIndexRef.current];
        btn?.focus();
      } else if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        const idx = focusIndexRef.current;
        if (idx >= 0 && idx < actionItems.length) {
          actionItems[idx].onSelect?.();
          onClose();
        }
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open, items, onClose]);

  const handleItemClick = (item: Extract<ContextMenuItem, { type?: "item" }>) => {
    if (!("disabled" in item && item.disabled)) {
      item.onSelect();
      onClose();
    }
  };

  if (!open || !position) return null;

  let actionIdx = -1;
  return createPortal(
    <div
      ref={menuRef}
      className="context-menu"
      role="menu"
      aria-label={ariaLabel}
      style={{
        position: "fixed",
        left: position.left,
        top: position.top,
        zIndex: "var(--z-menu, 96)",
        minWidth,
      }}
    >
      {items.map((item) => {
        if (item.type === "separator") {
          return <div key={item.key} className="context-menu__sep" role="separator" />;
        }
        actionIdx++;
        const idx = actionIdx;
        const disabled = item.disabled ?? false;
        return (
          <button
            key={item.key}
            ref={(el) => { itemRefs.current[idx] = el; }}
            className={`context-menu__item${item.danger ? " context-menu__item--danger" : ""}${item.variant === "section" ? " context-menu__item--section" : ""}`}
            role="menuitem"
            disabled={disabled}
            onClick={() => handleItemClick(item)}
          >
            {item.icon && <span className="context-menu__icon">{item.icon}</span>}
            <span className="context-menu__label">{item.label}</span>
          </button>
        );
      })}
    </div>,
    document.body,
  );
}
