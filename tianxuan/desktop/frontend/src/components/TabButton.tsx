import { memo } from "react";

export const TabButton = memo(function TabButton(p: {
  active: boolean;
  onClick: () => void;
  badge: number;
  children: string;
}) {
  return (
    <button
      className={`flex-1 px-4 py-2.5 text-[12.5px] font-medium border-0 bg-transparent cursor-pointer transition-colors border-b-2 ${
        p.active
          ? "text-accent border-accent"
          : "text-fg-faint border-transparent hover:text-fg-dim hover:border-fg-faint/30"
      }`}
      onClick={p.onClick}
      type="button"
    >
      {p.children}
      {p.badge > 0 && (
        <span className="ml-1.5 text-[10px] text-fg-faint">({p.badge})</span>
      )}
    </button>
  );
});
