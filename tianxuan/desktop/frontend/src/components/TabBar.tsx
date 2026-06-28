import type { TabMeta } from "../lib/types";

/** Minimal tab bar — one row of tabs above the main chat area. */
export function TabBar(p: {
  tabs: TabMeta[];
  activeTabId: string;
  onSelect: (id: string) => void;
}) {
  const { tabs, activeTabId, onSelect } = p;
  if (tabs.length <= 1) return null; // hide when only one tab

  return (
    <div className="flex items-center gap-0.5 px-2 py-1 border-b border-border-soft bg-bg overflow-x-auto no-drag">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-[12px] whitespace-nowrap border-0 cursor-pointer transition-colors ${
            tab.id === activeTabId
              ? "bg-accent/15 text-accent font-semibold"
              : "bg-transparent text-fg-faint hover:text-fg hover:bg-bg-soft"
          }`}
          onClick={() => onSelect(tab.id)}
          type="button"
          title={tab.workspaceRoot || tab.title}
        >
          {!tab.ready && (
            <span className="inline-block w-2 h-2 rounded-full bg-amber-400 animate-pulse" />
          )}
          <span className="max-w-[160px] truncate">{tab.title}</span>
        </button>
      ))}
    </div>
  );
}
